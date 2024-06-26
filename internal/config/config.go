package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.sr.ht/~mariusor/lw"
	"github.com/joho/godotenv"
)

var Prefix = "point"

type Env string

const (
	PROD Env = "prod"
	QA   Env = "qa"
	DEV  Env = "dev"
	TEST Env = "test"
)

var ValidEnvs = []Env{PROD, QA, DEV, TEST}

func ValidEnv(s string) bool {
	for _, v := range ValidEnvs {
		if strings.Contains(s, string(v)) {
			return true
		}
	}
	return false
}

func (e Env) IsProd() bool {
	return strings.Contains(string(e), string(PROD))
}

type BackendConfig struct {
	Enabled bool
	Host    string
	Port    int64
	User    string
	Pw      string
	Name    string
}

type StorageConfig struct {
	Type string
	Path string
}

type Options struct {
	Env       Env
	LogLevel  lw.Level
	LogOutput string
	TimeOut   time.Duration
	Secure    bool
	CertPath  string
	KeyPath   string
	Host      string
	Listen    string
	Storage   StorageConfig
}

type StorageType string

const (
	KeyENV         = "ENV"
	KeyTimeOut     = "TIME_OUT"
	KeyLogLevel    = "LOG_LEVEL"
	KeyLogOutput   = "LOG_OUTPUT"
	KeyHostname    = "HOSTNAME"
	KeyHTTPS       = "HTTPS"
	KeyCertPath    = "CERT_PATH"
	KeyKeyPath     = "KEY_PATH"
	KeyListen      = "LISTEN"
	KeyStorage     = "STORAGE"
	KeyStoragePath = "STORAGE_PATH"

	StorageFS     = "fs"
	StorageBoltDB = "boltdb"
	StorageBadger = "badger"
	StorageSqlite = "sqlite"
)

func prefKey(k string) string {
	if Prefix != "" {
		return fmt.Sprintf("%s_%s", strings.ToUpper(Prefix), k)
	}
	return k
}

func Getval(name, def string) string {
	val := def
	if pf := os.Getenv(prefKey(name)); len(pf) > 0 {
		val = pf
	}
	if p := os.Getenv(name); len(p) > 0 {
		val = p
	}
	return val
}

func LoadFromEnv(e Env, timeOut time.Duration) (Options, error) {
	conf := Options{}
	if !ValidEnv(string(e)) {
		e = Env(Getval(KeyENV, ""))
	}
	configs := []string{
		".env",
	}
	appendIfFile := func(typ Env) {
		envFile := fmt.Sprintf(".env.%s", typ)
		if _, err := os.Stat(envFile); err == nil {
			configs = append(configs, envFile)
		}
	}
	if !ValidEnv(string(e)) {
		for _, typ := range ValidEnvs {
			appendIfFile(typ)
		}
	} else {
		appendIfFile(e)
	}
	for _, f := range configs {
		godotenv.Load(f)
	}

	lvl := Getval(KeyLogLevel, "")
	switch strings.ToLower(lvl) {
	case "none":
		conf.LogLevel = lw.NoLevel
	case "trace":
		conf.LogLevel = lw.TraceLevel
	case "debug":
		conf.LogLevel = lw.DebugLevel
	case "warn":
		conf.LogLevel = lw.WarnLevel
	case "error":
		conf.LogLevel = lw.ErrorLevel
	case "info":
		fallthrough
	default:
		conf.LogLevel = lw.InfoLevel
	}
	conf.LogOutput = Getval(KeyLogOutput, "")

	if !ValidEnv(string(e)) {
		e = Env(Getval(KeyENV, "dev"))
	}
	conf.Env = e
	if conf.Host == "" {
		conf.Host = Getval(KeyHostname, conf.Host)
	}
	conf.TimeOut = timeOut
	if to, _ := time.ParseDuration(Getval(KeyTimeOut, "")); to > 0 {
		conf.TimeOut = to
	}
	conf.Secure, _ = strconv.ParseBool(Getval(KeyHTTPS, "false"))
	conf.KeyPath = Getval(KeyKeyPath, "")
	conf.CertPath = Getval(KeyCertPath, "")

	conf.Listen = Getval(KeyListen, "")
	typ := Getval(KeyStorage, "")
	path := Getval(KeyStoragePath, "")

	conf.Storage.Type = typ
	conf.Storage.Path = path
	conf.Storage.Path = normalizeStoragePath(path, conf.Storage, e)

	return conf, nil
}

func normalizeStoragePath(p string, o StorageConfig, env Env) string {
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

var ValidStorageTypes = []string{
	StorageFS, StorageBoltDB, StorageBadger, StorageSqlite,
}

func ValidStorageType(typ string) bool {
	for _, st := range ValidStorageTypes {
		if strings.EqualFold(st, typ) {
			return true
		}
	}
	return false
}

func ParseStorageDSN(s string) (string, string) {
	r := regexp.MustCompile(fmt.Sprintf(`(%s):\/\/(.+)`, strings.Join(ValidStorageTypes, "|")))
	found := r.FindAllSubmatch([]byte(s), -1)
	if len(found) == 0 {
		return DefaultStorage, ""
	}
	sto := found[0]
	if len(sto) == 1 {
		return DefaultStorage, string(sto[1])
	}
	return string(sto[1]), string(sto[2])
}
