//go:build storage_sqlite

package config

import (
	"git.sr.ht/~mariusor/lw"
	"github.com/go-ap/processing"
	sqlite "github.com/go-ap/storage-sqlite"
)

const DefaultStorage = StorageSqlite

func Storage(c Storage, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using sqlite Storage at %s", c.Path)
	return sqlite.New(sqlite.Config{
		Path:        c.Path,
		CacheEnable: true,
		LogFn:       l.Infof,
		ErrFn:       l.Errorf,
	})
}
