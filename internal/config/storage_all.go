//go:build storage_all || (!storage_fs && !storage_boltdb && !storage_badger && !storage_sqlite)

package config

import (
	"path/filepath"

	"git.sr.ht/~mariusor/lw"
	"github.com/go-ap/errors"
	"github.com/go-ap/processing"
	badger "github.com/go-ap/storage-badger"
	boltdb "github.com/go-ap/storage-boltdb"
	fs "github.com/go-ap/storage-fs"
	sqlite "github.com/go-ap/storage-sqlite"
)

const DefaultStorage = StorageFS

func getBadgerStorage(c Storage, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using badger Storage from %s", c.Path)
	return badger.New(badger.Config{
		Path:        c.Path,
		CacheEnable: false,
		LogFn:       l.Debugf,
		ErrFn:       l.Warnf,
	})
}

func getBoltStorage(c Storage, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using boltdb Storage from %s", c.Path)
	return boltdb.New(boltdb.Config{
		Path:  c.Path,
		LogFn: l.Infof,
		ErrFn: l.Errorf,
	})
}

func getSqliteStorage(c Storage, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using sqlite Storage at %s", c.Path)
	return sqlite.New(sqlite.Config{
		Path:        c.Path,
		CacheEnable: true,
		LogFn:       l.Infof,
		ErrFn:       l.Errorf,
	})
}

func getFsStorage(c Storage, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using fs Storage at %s", c.Path)
	return fs.New(fs.Config{
		Path:        filepath.Dir(c.Path),
		CacheEnable: true,
		LogFn:       l.Infof,
		ErrFn:       l.Errorf,
	})
}

func NewStorage(c Storage, l lw.Logger) (processing.Store, error) {
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
