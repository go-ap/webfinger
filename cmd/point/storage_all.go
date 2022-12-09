//go:build !storage_pgx && !storage_boltdb && !storage_fs && !storage_badger && !storage_sqlite

package main

import (
	"path/filepath"
	"strings"

	"git.sr.ht/~mariusor/lw"
	"github.com/go-ap/errors"
	"github.com/go-ap/processing"
	badger "github.com/go-ap/storage-badger"
	boltdb "github.com/go-ap/storage-boltdb"
	fs "github.com/go-ap/storage-fs"
	sqlite "github.com/go-ap/storage-sqlite"
)

func getBadgerStorage(c Config, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using badger storage from %s", c.Path)
	return badger.New(badger.Config{
		Path:        c.Path,
		CacheEnable: false,
		LogFn:       l.Debugf,
		ErrFn:       l.Warnf,
	})
}

func boltLogFn(logFn func(string, ...any)) func(lw.Ctx, string, ...interface{}) {
	return func(ctx lw.Ctx, s string, i ...interface{}) {
		ss := strings.Builder{}
		ss.WriteString(s)
		ss.WriteRune(' ')
		for k, v := range ctx {
			ss.WriteString("%s=%+v")
			i = append(i, k, v)
			ss.WriteRune(' ')
		}
		logFn(ss.String(), i...)
	}
}

func getBoltStorage(c Config, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using boltdb storage from %s", c.Path)
	return boltdb.New(boltdb.Config{
		Path:  c.Path,
		LogFn: l.Infof,
		ErrFn: l.Errorf,
	})
}

func getSqliteStorage(c Config, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using sqlite storage at %s", c.Path)
	return sqlite.New(sqlite.Config{
		Path:        c.Path,
		CacheEnable: true,
	})
}

func getFsStorage(c Config, l lw.Logger) (processing.Store, error) {
	l.Debugf("Using fs storage at %s", c.Path)
	return fs.New(fs.Config{
		Path:        filepath.Dir(c.Path),
		CacheEnable: true,
	})
}

func Storage(c Config, l lw.Logger) (processing.Store, error) {
	switch c.Storage {
	case StorageBoltDB:
		return getBoltStorage(c, l)
	case StorageBadger:
		return getBadgerStorage(c, l)
	case StorageSqlite:
		return getSqliteStorage(c, l)
	case StorageFS:
		return getFsStorage(c, l)
	}
	return nil, errors.NotImplementedf("Invalid storage type %s", c.Storage)
}
