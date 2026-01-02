package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"git.sr.ht/~mariusor/lw"
	w "git.sr.ht/~mariusor/wrapper"
	"github.com/alecthomas/kong"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/go-ap/webfinger"
	"github.com/go-ap/webfinger/internal/config"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"
)

var Point struct {
	ListenOn string   `name:"listen" help:"The socket to listen on."`
	Env      string   `name:"env" help:"Environment type: ${env_types}" default:"${default_env}"`
	KeyPath  string   `name:"key-path" help:"SSL key path for HTTPS." type:"path"`
	CertPath string   `name:"cert-path" help:"SSL cert path for HTTPS." type:"path"`
	Root     []string `name:"root" help:"Root actor IRI for Storage" group:"config-options"`
	Config   []string `name:"config" help:"Configuration path for .env file" group:"config-options" xor:"config-options"`
	Storage  []string `name:"storage" help:"Storage DSN strings of form type:///path/to/storage." group:"config-options" xor:"config-options"`
}

var l = lw.Dev()

var defaultTimeout = time.Second * 10

var version = "HEAD"

const defaultGraceWait = 1500 * time.Millisecond

func main() {
	ktx := kong.Parse(
		&Point,
		kong.Bind(l),
		kong.Vars{
			"env_types":   strings.Join([]string{string(config.DEV), string(config.PROD)}, ", "),
			"default_env": string(config.DEV),
		},
	)
	env := config.DEV
	if config.ValidEnv(Point.Env) {
		env = config.Env(Point.Env)
	}

	errors.SetIncludeBacktrace(!env.IsProd())

	if build, ok := debug.ReadBuildInfo(); ok && version == "HEAD" && build.Main.Version != "(devel)" {
		version = build.Main.Version
	}

	var stores []webfinger.Storage
	var err error

	if len(Point.Storage) > 0 && len(Point.Root) > 0 {
		if stores, err = loadStoresFromDSNs(Point.Storage, Point.Root, env, l.WithContext(lw.Ctx{"log": "storage"})); err != nil {
			l.Errorf("Errors loading storage paths: %+s", err)
		}
	}
	if len(Point.Config) > 0 && len(Point.Root) > 0 {
		if stores, err = loadStoresFromConfigs(Point.Config, Point.Root, env, l.WithContext(lw.Ctx{"log": "storage"})); err != nil {
			l.Errorf("Errors loading config files: %+s", err)
		}
	}
	if err != nil {
		os.Exit(1)
	}

	if len(stores) == 0 {
		l.Errorf("Unable to find any valid storage path")
		os.Exit(1)
	}

	defer func() {
		for _, st := range stores {
			st.Close()
		}
	}()

	m := http.NewServeMux()

	h := webfinger.New(l, stores...)

	logCtx := lw.Ctx{
		"version":  version,
		"listenOn": Point.ListenOn,
	}
	l = l.WithContext(logCtx)

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"https://*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
		MaxAge:           int(time.Hour.Seconds()),
	})
	c.Log = corsLogger(l.WithContext(lw.Ctx{"log": "cors"}).Tracef)

	wrappedHandler := func(next func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
		return c.Handler(http.HandlerFunc(next)).ServeHTTP
	}

	m.HandleFunc(webfinger.WellKnownOAuthAuthorizationServerPath, wrappedHandler(h.HandleOAuthAuthorizationServer))
	m.HandleFunc(webfinger.WellKnownOAuthAuthorizationServerPath+"/", wrappedHandler(h.HandleOAuthAuthorizationServer))
	m.HandleFunc(webfinger.WellKnownWebFingerPath, h.HandleWebFinger)
	m.HandleFunc(webfinger.WellKnownHostPath, h.HandleHostMeta)

	setters := []w.SetFn{w.Handler(m), w.GracefulWait(defaultGraceWait)}

	if len(Point.CertPath)+len(Point.KeyPath) > 0 {
		setters = append(setters, w.WithTLSCert(Point.CertPath, Point.KeyPath))
	}
	dir, _ := filepath.Split(Point.ListenOn)
	if Point.ListenOn == "systemd" {
		setters = append(setters, w.OnSystemd())
	} else if _, err := os.Stat(dir); err == nil {
		setters = append(setters, w.OnSocket(Point.ListenOn))
		defer func() { os.RemoveAll(Point.ListenOn) }()
	} else {
		setters = append(setters, w.OnTCP(Point.ListenOn))
	}

	ctx, cancelFn := context.WithCancel(context.TODO())
	defer cancelFn()

	// Get start/stop functions for the http server
	srvRun, srvStop := w.HttpServer(setters...)
	stopFn := func(ctx context.Context) error {
		l.Infof("Shutting down")
		for _, st := range stores {
			st.Close()
		}
		return srvStop(ctx)
	}

	exitWithErrOrInterrupt := func(err error, exit chan<- error) {
		if err == nil {
			err = w.Interrupt
		}
		exit <- err
	}

	l.Infof("Listening for .well-known requests")
	err = w.RegisterSignalHandlers(w.SignalHandlers{
		syscall.SIGHUP: func(_ chan<- error) {
			l.Debugf("SIGHUP received, reloading configuration")
		},
		syscall.SIGINT: func(exit chan<- error) {
			l.WithContext(lw.Ctx{"wait": defaultGraceWait}).Infof("SIGINT received, stopping")
			exitWithErrOrInterrupt(stopFn(ctx), exit)
		},
		syscall.SIGTERM: func(exit chan<- error) {
			l.WithContext(lw.Ctx{"wait": defaultGraceWait}).Infof("SIGTERM received, force stopping")
			exitWithErrOrInterrupt(stopFn(ctx), exit)
		},
		syscall.SIGQUIT: func(exit chan<- error) {
			l.Infof("SIGQUIT received, ungraceful force stopping")
			cancelFn()
			exitWithErrOrInterrupt(stopFn(ctx), exit)
		},
	}).Exec(ctx, srvRun)

	if err != nil {
		l.Errorf("Error: %+s", err)
		ktx.Exit(1)
	}

	ktx.Exit(0)
}

func loadStoresFromDSNs(dsns, root []string, env config.Env, l lw.Logger) ([]webfinger.Storage, error) {
	stores := make([]webfinger.Storage, 0)
	errs := make([]error, 0)
	for _, sto := range dsns {
		typ, path := config.ParseStorageDSN(sto)

		if !config.ValidStorageType(typ) {
			typ = config.DefaultStorage
			path = sto
		}
		conf := config.StorageConfig{Type: typ, Path: path}
		db, err := config.Storage(conf, env, l)
		if err != nil {
			errs = append(errs, fmt.Errorf("unable to initialize storage backend [%s]%s: %w", typ, path, err))
			continue
		}
		fs, ok := db.(webfinger.Store)
		if !ok {
			errs = append(errs, fmt.Errorf("invalid storage backend %T [%s]%s", db, typ, path))
			continue
		}
		if err = fs.Open(); err != nil {
			errs = append(errs, fmt.Errorf("unable to open storage backend %T [%s]%s", db, typ, path))
			continue
		}
		for _, iri := range root {
			if actor, err := db.Load(vocab.IRI(iri)); err == nil {
				if app, err := vocab.ToActor(actor); err == nil {
					s := webfinger.Storage{Store: fs, Root: *app}
					stores = append(stores, s)
				}
			}
		}
	}
	return stores, errors.Join(errs...)
}

type corsLogger func(string, ...any)

func (c corsLogger) Printf(f string, v ...interface{}) {
	c(f, v...)
}

func loadStoresFromConfigs(paths, root []string, env config.Env, l lw.Logger) ([]webfinger.Storage, error) {
	stores := make([]webfinger.Storage, 0)
	errs := make([]error, 0)
	for _, p := range paths {
		if err := godotenv.Load(p); err != nil {
			errs = append(errs, err)
			continue
		}
		opts, err := config.LoadFromEnv(env, defaultTimeout)
		if err != nil {
			errs = append(errs, fmt.Errorf("unable to load configuration %s: %w", p, err))
			continue
		}

		if opts.Listen != "" && Point.ListenOn == "" {
			Point.ListenOn = opts.Listen
		}

		st := opts.Storage
		db, err := config.Storage(opts.Storage, opts.Env, l)
		if err != nil {
			errs = append(errs, fmt.Errorf("unable to initialize storage backend [%s]%s: %w", st.Type, st.Path, err))
			continue
		}
		fs, ok := db.(webfinger.Store)
		if !ok {
			errs = append(errs, fmt.Errorf("invalid storage backend %T [%s]%s", db, st.Type, st.Path))
			continue
		}
		if err = fs.Open(); err != nil {
			errs = append(errs, fmt.Errorf("unable to open storage backend %T [%s]%s", db, st.Type, st.Path))
			continue
		}
		for _, iri := range root {
			if actor, err := db.Load(vocab.IRI(iri)); err == nil {
				if app, err := vocab.ToActor(actor); err == nil {
					s := webfinger.Storage{Store: fs, Root: *app}
					stores = append(stores, s)
				}
			}
		}
	}
	return stores, errors.Join(errs...)
}
