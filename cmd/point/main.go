package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"git.sr.ht/~mariusor/lw"
	w "git.sr.ht/~mariusor/wrapper"
	"github.com/alecthomas/kong"
	"github.com/go-ap/processing"
	"github.com/go-ap/webfinger"
	"github.com/go-ap/webfinger/internal/config"
)

var Point struct {
	ListenOn string   `required:"" name:"listen" help:"The socket to listen on." default:"localhost:3666"`
	KeyPath  string   `name:"key-path" help:"SSL key path for HTTPS." type:"path"`
	CertPath string   `name:"cert-path" help:"SSL cert path for HTTPS." type:"path"`
	Storage  []string `required:"" flag:"" name:"storage" help:"Storage DSN strings of form type:/path/to/storage."`
}

const (
	StorageBoltDB = "boltdb"
	StorageBadger = "badger"
	StorageSqlite = "sqlite"
	StorageFS     = "fs"
)

var l = lw.Dev()

type Config struct {
	Storage string
	Path    string
}

func exit(errs ...error) {
	if len(errs) == 0 {
		os.Exit(0)
		return
	}
	for _, err := range errs {
		l.Errorf("%s", err)
	}
	os.Exit(-1)
}

func main() {
	ktx := kong.Parse(&Point, kong.Bind(l))

	stores := make([]processing.ReadStore, 0)
	for _, sto := range Point.Storage {
		typ, path := config.ParseStorageDsn(sto)

		if !config.ValidStorageType(typ) {
			typ = config.DefaultStorage
			path = sto
		}
		conf := config.Storage{Type: typ, Path: path}
		db, err := config.NewStorage(conf, l)
		if err != nil {
			exit(fmt.Errorf("unable to initialize storage backend: %w", err))
			return
		}
		stores = append(stores, db)
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

	ctx, cancelFn := context.WithTimeout(context.TODO(), time.Second*10)
	defer cancelFn()

	// Get start/stop functions for the http server
	srvRun, srvStop := w.HttpServer(setters...)
	l.Infof("Listening for webfinger requests on %s", Point.ListenOn)
	stopFn := func() {
		if err := srvStop(ctx); err != nil {
			l.Errorf("%s", err)
		}
	}

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
			l.Errorf("%s", err)
			return err
		}
		var err error
		// Doesn't block if no connections, but will otherwise wait until the timeout deadline.
		go func(e error) {
			l.Errorf("%s", err)
			stopFn()
		}(err)
		return err
	})
	if exit == 0 {
		l.Infof("Shutting down")
	}

	ktx.Exit(exit)
}
