//go:build storage_sqlite

package main

import (
	"git.sr.ht/~mariusor/lw"
	"github.com/go-ap/processing"
	sqlite "github.com/go-ap/storage-sqlite"
)

func Storage(c Config, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using sqlite storage at %s", c.Path)
	return sqlite.New(sqlite.Config{
		Path:        c.Path,
		URL:         c.BaseURL,
		CacheEnable: true,
	})
}
