//go:build storage_badger

package config

import (
	"git.sr.ht/~mariusor/lw"
	"github.com/go-ap/processing"
	badger "github.com/go-ap/storage-badger"
)

const DefaultStorage = StorageBadger

func Storage(c StorageConfig, env Env, l lw.Logger) (processing.ReadStore, error) {
	c.Path = normalizeStoragePath(c.Path, c, env)
	l.Debugf("Using badger storage from %s", c.Path)
	return badger.New(badger.Config{
		Path:        c.Path,
		CacheEnable: false,
		LogFn:       l.Debugf,
		ErrFn:       l.Warnf,
	})
}
