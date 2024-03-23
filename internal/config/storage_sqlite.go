//go:build storage_sqlite

package config

import (
	"git.sr.ht/~mariusor/lw"
	sqlite "github.com/go-ap/storage-sqlite"
	"github.com/go-ap/webfinger"
)

const DefaultStorage = StorageSqlite

func Storage(c StorageConfig, env Env, l lw.Logger) (webfinger.FullStorage, error) {
	c.Path = normalizeStoragePath(c.Path, c, env)
	l.Debugf("Using sqlite storage at %s", c.Path)
	return sqlite.New(sqlite.Config{
		Path:        c.Path,
		CacheEnable: true,
		LogFn:       l.Infof,
		ErrFn:       l.Errorf,
	})
}
