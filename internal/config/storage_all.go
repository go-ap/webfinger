//go:build storage_all || (!storage_fs && !storage_boltdb && !storage_badger && !storage_sqlite)

package config

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"git.sr.ht/~mariusor/lw"
	"github.com/go-ap/errors"
	badger "github.com/go-ap/storage-badger"
	boltdb "github.com/go-ap/storage-boltdb"
	fs "github.com/go-ap/storage-fs"
	sqlite "github.com/go-ap/storage-sqlite"
	"github.com/go-ap/webfinger"
)

const DefaultStorage = StorageFS

func getBadgerStorage(c Storage, l lw.Logger) (webfinger.FullStorage, error) {
	l.Debugf("Using badger Storage from %s", c.Path)
	return badger.New(badger.Config{
		Path:        c.Path,
		CacheEnable: false,
		LogFn:       l.Debugf,
		ErrFn:       l.Warnf,
	})
}

func getBoltStorage(c Storage, l lw.Logger) (webfinger.FullStorage, error) {
	l.Debugf("Using boltdb Storage from %s", c.Path)
	return boltdb.New(boltdb.Config{
		Path:  c.Path,
		LogFn: l.Infof,
		ErrFn: l.Errorf,
	})
}

func getSqliteStorage(c Storage, l lw.Logger) (webfinger.FullStorage, error) {
	l.Debugf("Using sqlite Storage at %s", c.Path)
	return sqlite.New(sqlite.Config{
		Path:        c.Path,
		CacheEnable: true,
		LogFn:       l.Infof,
		ErrFn:       l.Errorf,
	})
}

func getFsStorage(c Storage, l lw.Logger) (webfinger.FullStorage, error) {
	l.Debugf("Using fs Storage at %s", c.Path)
	return fs.New(fs.Config{
		Path:        c.Path,
		CacheEnable: true,
		Logger:      l,
	})
}

func normalizeStoragePath(p string, o Storage, env Env) string {
	if len(p) == 0 {
		return p
	}
	if p[0] == '~' {
		p = os.Getenv("HOME") + p[1:]
	}
	if !filepath.IsAbs(p) {
		p, _ = filepath.Abs(p)
	}
	p = strings.ReplaceAll(p, "%env%", string(env))
	p = strings.ReplaceAll(p, "%storage%", o.Type)
	return path.Clean(p)
}

func NewStorage(c Storage, env Env, l lw.Logger) (webfinger.FullStorage, error) {
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
