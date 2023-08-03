//go:build storage_badger

package config

import (
	"git.sr.ht/~mariusor/lw"
	"github.com/go-ap/processing"
	badger "github.com/go-ap/storage-badger"
)

const DefaultStorage = StorageBadger

func Storage(c Storage, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using badger Storage from %s", c.Path)
	return badger.New(badger.Config{
		Path:        c.Path,
		CacheEnable: false,
		LogFn:       l.Debugf,
		ErrFn:       l.Warnf,
	})
}
