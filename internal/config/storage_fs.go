//go:build storage_fs

package config

import (
	"git.sr.ht/~mariusor/lw"
	"github.com/go-ap/processing"
	fs "github.com/go-ap/storage-fs"
)

const DefaultStorage = StorageFS

func Storage(c Storage, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using fs Storage from %s", c.Path)
	return fs.New(fs.Config{
		Path:        c.Path,
		CacheEnable: false,
		LogFn:       l.Infof,
		ErrFn:       l.Errorf,
	})
}
