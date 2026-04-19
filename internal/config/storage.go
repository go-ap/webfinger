package config

import (
	"git.sr.ht/~mariusor/lw"
	"git.sr.ht/~mariusor/storage-all"
)

const DefaultStorage = StorageFS

func Storage(c StorageConfig, env Env, l lw.Logger) (storage.FullStorage, error) {
	return storage.New(
		storage.WithPath(c.Path),
		storage.WithType(storage.Type(c.Type)),
		storage.WithEnv(string(env)),
		storage.WithLogger(l),
	)
}
