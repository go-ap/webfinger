//go:build storage_boltdb

package config

import (
	"git.sr.ht/~mariusor/lw"
	"github.com/go-ap/processing"
	boltdb "github.com/go-ap/storage-boltdb"
)

const DefaultStorage = StorageBoltDB

func Storage(c Storage, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using boltdb Storage from %s", c.Path)
	return boltdb.New(boltdb.Config{
		Path:  c.Path,
		LogFn: l.Infof,
		ErrFn: l.Errorf,
	})
}
