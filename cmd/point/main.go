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
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/webfinger"
	"github.com/writeas/go-nodeinfo"
)

var listenOn string = "localhost:3666"
var baseURL string
var certPath string
var keyPath string
var storagePath string

const (
	StorageBoltDB = "boltdb"
	StorageBadger = "badger"
	StorageSqlite = "sqlite"
	StorageFS     = "fs"
)

var l = lw.Dev(lw.SetLevel(lw.DebugLevel), lw.SetOutput(os.Stderr))

type Config struct {
	Storage string
	Path    string
	BaseURL string
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
	if len(os.Args) < 3 {
		exit(fmt.Errorf("need to pass instance URL and storage path to the application"))
		return
	}
	baseURL = os.Args[1]
	storagePath = os.Args[2]
	conf := Config{
		Storage: StorageFS,
		BaseURL: baseURL,
		Path:    storagePath,
	}
	db, err := Storage(conf, l)
	if err != nil {
		exit(fmt.Errorf("unable to initialize storage backend: %w", err))
		return
	}

	res, err := db.Load(vocab.IRI(baseURL))
	if err != nil {
		exit(fmt.Errorf("unable to load application"))
		return
	}
	var app vocab.Actor
	err = vocab.OnActor(res, func(actor *vocab.Actor) error {
		app = *actor
		return nil
	})
	if err != nil {
		exit(fmt.Errorf("unable to load instance Service: %w", err))
		return
	}
	if app.ID == "" {
		exit(fmt.Errorf("instance Service was not found in storage"))
		return
	}

	m := http.NewServeMux()
	cfg := webfinger.NodeInfoConfig(app, webfinger.WebInfo{})
	ni := nodeinfo.NewService(cfg, webfinger.NodeInfoResolverNew(db, app))

	h := webfinger.New(app, db)
	m.HandleFunc("/.well-known/webfinger", h.HandleWebFinger)
	m.HandleFunc("/.well-known/host-meta", h.HandleHostMeta)
	m.HandleFunc("/.well-known/nodeinfo", ni.NodeInfoDiscover)
	m.HandleFunc("/nodeinfo", ni.NodeInfo)

	setters := []w.SetFn{w.Handler(m)}
	dir, _ := filepath.Split(listenOn)
	if listenOn == "systemd" {
		setters = append(setters, w.Systemd())
	} else if _, err := os.Stat(dir); err == nil {
		setters = append(setters, w.Socket(listenOn))
		defer func() { os.RemoveAll(listenOn) }()
	} else if len(certPath)+len(keyPath) > 0 {
		setters = append(setters, w.HTTPS(listenOn, certPath, keyPath))
	} else {
		setters = append(setters, w.HTTP(listenOn))
	}

	ctx, cancelFn := context.WithTimeout(context.TODO(), time.Second*10)
	defer cancelFn()

	// Get start/stop functions for the http server
	srvRun, srvStop := w.HttpServer(setters...)
	l.Infof("Started %s %s", baseURL, listenOn)
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

	os.Exit(exit)
}
