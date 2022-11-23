//go:build storage_badger

package main

import (
	"git.sr.ht/~mariusor/lw"
	"github.com/go-ap/processing"
	"github.com/go-ap/storage-badger"
)

func Storage(c Config, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using badger storage from %s", c.Path)
	return badger.New(badger.Config{
		Path:        c.Path,
		CacheEnable: false,
	})
}
