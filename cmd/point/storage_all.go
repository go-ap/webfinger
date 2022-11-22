//go:build !storage_pgx && !storage_boltdb && !storage_fs && !storage_badger && !storage_sqlite

package main

import (
	"path/filepath"

	"git.sr.ht/~mariusor/lw"
	"github.com/go-ap/errors"
	"github.com/go-ap/processing"
	//badger "github.com/go-ap/storage-badger"
	//boltdb "github.com/go-ap/storage-boltdb"
	fs "github.com/go-ap/storage-fs"
	//sqlite "github.com/go-ap/storage-sqlite"
)

/*
func getBadgerStorage(c Config, l lw.Logger) (processing.Store, error) {
	conf := badger.Config{
		Path:    c.Path,
		URL: c.URL,
		Logger:  l,
	}
	if l != nil {
		l.Debugf("Using badger storage at %s", c.Path)
	}
	db, err := badger.New(conf)
	if err != nil {
		return db, err
	}
}

func getBoltStorage(c Config, l lw.Logger) (processing.Store, error) {
	path := c.BaseStoragePath()
	l.Debugf("Using boltdb storage at %s", path)
	return boltdb.New(boltdb.Config{
		Path:    path,
		URL: c.URL,
		LogFn:   InfoLogFn(l),
		ErrFn:   ErrLogFn(l),
	})
}

func getSqliteStorage(c Config, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using sqlite storage at %s", path)
	return sqlite.New(sqlite.Config{
		Path: c.Path,
		URL:     c.URL,
		CacheEnable: true,
	})
}

*/

func getFsStorage(c Config, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using fs storage at %s", c.Path)
	return fs.New(fs.Config{
		Path:        filepath.Dir(c.Path),
		URL:         c.BaseURL,
		CacheEnable: true,
	})
}

func Storage(c Config, l lw.Logger) (processing.Store, error) {
	switch c.Storage {
	//case StorageBoltDB:
	//	return getBoltStorage(c, l)
	//case StorageBadger:
	//	return getBadgerStorage(c, l)
	//case StorageSqlite:
	//	return getSqliteStorage(c, l)
	case StorageFS:
		return getFsStorage(c, l)
	}
	return nil, errors.NotImplementedf("Invalid storage type %s", c.Storage)
}
