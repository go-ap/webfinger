//go:build storage_boltdb

package config

import (
	"git.sr.ht/~mariusor/lw"
	boltdb "github.com/go-ap/storage-boltdb"
	"github.com/go-ap/webfinger"
)

const DefaultStorage = StorageBoltDB

func Storage(c StorageConfig, env Env, l lw.Logger) (webfinger.FullStorage, error) {
	c.Path = normalizeStoragePath(c.Path, c, env)
	l.Debugf("Using boltdb storage from %s", c.Path)
	return boltdb.New(boltdb.Config{
		Path:  c.Path,
		LogFn: l.Infof,
		ErrFn: l.Errorf,
	})
}
