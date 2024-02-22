package config

import (
	"fmt"
	"os"
	"testing"
	"time"
)

const (
	hostname = "testing.git"
	logLvl   = "panic"
	secure   = true
	listen   = "127.0.0.3:666"
	pgSQL    = "postgres"
)

func TestLoadFromEnv(t *testing.T) {
	{
		os.Setenv(KeyHostname, hostname)
		os.Setenv(KeyLogLevel, logLvl)
		os.Setenv(KeyHTTPS, fmt.Sprintf("%t", secure))
		os.Setenv(KeyListen, listen)
		os.Setenv(KeyStorage, pgSQL)

		c, err := LoadFromEnv(TEST, time.Second)
		if err != nil {
			t.Errorf("Error loading env: %s", err)
		}

		if c.Host != hostname {
			t.Errorf("Invalid loaded value for %s: %s, expected %s", KeyHostname, c.Host, hostname)
		}
		if c.Secure != secure {
			t.Errorf("Invalid loaded value for %s: %t, expected %t", KeyHTTPS, c.Secure, secure)
		}
		if c.Listen != listen {
			t.Errorf("Invalid loaded value for %s: %s, expected %s", KeyListen, c.Listen, listen)
		}
	}
}
