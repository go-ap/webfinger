//go:build storage_all || (!storage_fs && !storage_boltdb && !storage_badger && !storage_sqlite)

package config

import (
	"git.sr.ht/~mariusor/lw"
	"github.com/go-ap/errors"
	badger "github.com/go-ap/storage-badger"
	boltdb "github.com/go-ap/storage-boltdb"
	fs "github.com/go-ap/storage-fs"
	sqlite "github.com/go-ap/storage-sqlite"
	"github.com/go-ap/webfinger"
)

const DefaultStorage = StorageFS

func getBadgerStorage(c StorageConfig, l lw.Logger) (webfinger.FullStorage, error) {
	l.Debugf("Using badger storage from %s", c.Path)
	return badger.New(badger.Config{
		Path:        c.Path,
		CacheEnable: false,
		LogFn:       l.Debugf,
		ErrFn:       l.Warnf,
	})
}

func getBoltStorage(c StorageConfig, l lw.Logger) (webfinger.FullStorage, error) {
	l.Debugf("Using boltdb storage from %s", c.Path)
	return boltdb.New(boltdb.Config{
		Path:  c.Path,
		LogFn: l.Infof,
		ErrFn: l.Errorf,
	})
}

func getSqliteStorage(c StorageConfig, l lw.Logger) (webfinger.FullStorage, error) {
	l.Debugf("Using sqlite storage at %s", c.Path)
	return sqlite.New(sqlite.Config{
		Path:        c.Path,
		CacheEnable: true,
		LogFn:       l.Infof,
		ErrFn:       l.Errorf,
	})
}

func getFsStorage(c StorageConfig, l lw.Logger) (webfinger.FullStorage, error) {
	l.Debugf("Using fs storage at %s", c.Path)
	return fs.New(fs.Config{
		Path:        c.Path,
		CacheEnable: true,
		Logger:      l,
	})
}

func Storage(c StorageConfig, env Env, l lw.Logger) (webfinger.FullStorage, error) {
	c.Path = normalizeStoragePath(c.Path, c, env)
	switch c.Type {
	case StorageBoltDB:
		return getBoltStorage(c, l)
	case StorageBadger:
		return getBadgerStorage(c, l)
	case StorageSqlite:
		return getSqliteStorage(c, l)
	case StorageFS:
		return getFsStorage(c, l)
	}
	return nil, errors.NotImplementedf("Invalid Storage type %s", c.Type)
}
