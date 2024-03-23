//go:build storage_fs

package config

import (
	"git.sr.ht/~mariusor/lw"
	fs "github.com/go-ap/storage-fs"
	"github.com/go-ap/webfinger"
)

const DefaultStorage = StorageFS

func Storage(c StorageConfig, env Env, l lw.Logger) (webfinger.FullStorage, error) {
	c.Path = normalizeStoragePath(c.Path, c, env)
	l.Debugf("Using fs storage from %s", c.Path)
	return fs.New(fs.Config{
		Path:        c.Path,
		CacheEnable: false,
		Logger:      l,
	})
}
