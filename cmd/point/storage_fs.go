//go:build storage_fs

package main

import (
	"git.sr.ht/~mariusor/lw"
	"github.com/go-ap/processing"
	fs "github.com/go-ap/storage-fs"
)

func Storage(c Config, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using fs storage from %s", c.Path)
	return fs.New(fs.Config{
		Path:        c.Path,
		URL:         c.BaseURL,
		CacheEnable: false,
	})
}
