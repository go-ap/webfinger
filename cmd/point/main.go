package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"git.sr.ht/~mariusor/lw"
	w "git.sr.ht/~mariusor/wrapper"
	"github.com/alecthomas/kong"
	"github.com/go-ap/webfinger"
	"github.com/go-ap/webfinger/internal/config"
	"github.com/joho/godotenv"
)

var Point struct {
	ListenOn string   `name:"listen" help:"The socket to listen on."`
	Env      string   `name:"env" help:"Environment type: ${env_types}" default:"${default_env}"`
	KeyPath  string   `name:"key-path" help:"SSL key path for HTTPS." type:"path"`
	CertPath string   `name:"cert-path" help:"SSL cert path for HTTPS." type:"path"`
	Config   []string `name:"config" help:"Configuration path for .env file" group:"config-options" xor:"config-options"`
	Storage  []string `name:"storage" help:"Storage DSN strings of form type:///path/to/storage." group:"config-options" xor:"config-options"`
}

var l = lw.Dev()

type Config struct {
	Storage string
	Path    string
}

var defaultTimeout = time.Second * 10

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

	var stores []webfinger.FullStorage
	var err error
	if len(Point.Storage) > 0 {
		if stores, err = loadStoresFromDSNs(Point.Storage, env, l.WithContext(lw.Ctx{"log": "storage"})); err != nil {
			l.Errorf("Errors loading storage paths: %+s", err)
		}
	}
	if len(Point.Config) > 0 {
		if stores, err = loadStoresFromConfigs(Point.Config, env, l.WithContext(lw.Ctx{"log": "storage"})); err != nil {
			l.Errorf("Errors loading config files: %+s", err)
		}
	}
	if err != nil {
		os.Exit(1)
	}

	m := http.NewServeMux()
	h := webfinger.New(l, stores...)
	m.HandleFunc("/.well-known/webfinger", h.HandleWebFinger)
	m.HandleFunc("/.well-known/host-meta", h.HandleHostMeta)

	setters := []w.SetFn{w.Handler(m)}

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

	ctx, cancelFn := context.WithTimeout(context.TODO(), defaultTimeout)
	defer cancelFn()

	// Get start/stop functions for the http server
	srvRun, srvStop := w.HttpServer(setters...)
	l.Infof("Listening for webfinger requests on %s", Point.ListenOn)
	stopFn := func() {
		if err := srvStop(ctx); err != nil {
			l.Errorf("%+v", err)
		}
	}
	defer stopFn()

	exit := w.RegisterSignalHandlers(w.SignalHandlers{
		syscall.SIGHUP: func(_ chan int) {
			l.Infof("SIGHUP received, reloading configuration")
		},
		syscall.SIGINT: func(exit chan int) {
			l.Infof("SIGINT received, stopping")
			exit <- 0
		},
		syscall.SIGTERM: func(exit chan int) {
			l.Infof("SIGITERM received, force stopping")
			exit <- 0
		},
		syscall.SIGQUIT: func(exit chan int) {
			l.Infof("SIGQUIT received, force stopping with core-dump")
			exit <- 0
		},
	}).Exec(func() error {
		if err := srvRun(); err != nil {
			l.Errorf("%+v", err)
			return err
		}
		return nil
	})
	if exit == 0 {
		l.Infof("Shutting down")
	}

	ktx.Exit(exit)
}

func loadStoresFromDSNs(dsns []string, env config.Env, l lw.Logger) ([]webfinger.FullStorage, error) {
	stores := make([]webfinger.FullStorage, 0)
	errs := make([]error, 0)
	for _, sto := range dsns {
		typ, path := config.ParseStorageDSN(sto)

		if !config.ValidStorageType(typ) {
			typ = config.DefaultStorage
			path = sto
		}
		conf := config.Storage{Type: typ, Path: path}
		db, err := config.NewStorage(conf, env, l)
		if err != nil {
			errs = append(errs, fmt.Errorf("unable to initialize storage backend [%s]%s: %w", typ, path, err))
			continue
		}
		fs, ok := db.(webfinger.FullStorage)
		if !ok {
			errs = append(errs, fmt.Errorf("invalid storage backend %T [%s]%s", db, typ, path))
			continue
		}
		stores = append(stores, fs)
	}
	return stores, errors.Join(errs...)
}

func loadStoresFromConfigs(paths []string, env config.Env, l lw.Logger) ([]webfinger.FullStorage, error) {
	stores := make([]webfinger.FullStorage, 0)
	errs := make([]error, 0)
	config.Prefix = "fedbox"
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
		db, err := config.NewStorage(opts.Storage, opts.Env, l)
		if err != nil {
			errs = append(errs, fmt.Errorf("unable to initialize storage backend [%s]%s: %w", st.Type, st.Path, err))
			continue
		}
		fs, ok := db.(webfinger.FullStorage)
		if !ok {
			errs = append(errs, fmt.Errorf("invalid storage backend %T [%s]%s", db, st.Type, st.Path))
			continue
		}
		stores = append(stores, fs)
	}
	return stores, errors.Join(errs...)
}
