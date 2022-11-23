//go:build storage_boltdb

package main

import (
	"git.sr.ht/~mariusor/lw"
	"github.com/go-ap/processing"
	"github.com/go-ap/storage-boltdb"
)

func Storage(c Config, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using boltdb storage from %s", c.Path)
	return boltdb.New(boltdb.Config{Path: c.Path})
}
