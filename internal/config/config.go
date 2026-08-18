package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr          string
	DataDir       string
	IngestSecret  string
	Window        time.Duration
	IdemTTL       time.Duration
	Workers       int
	PublicBase    string
	LoopbackPath  string
}

func Load() (Config, error) {
	c := Config{
		Addr:         env("HOOKRELAY_ADDR", ":8080"),
		DataDir:      env("HOOKRELAY_DATA_DIR", "./data"),
		IngestSecret: env("HOOKRELAY_INGEST_SECRET", "dev-ingest-secret"),
		Window:       durSec("HOOKRELAY_WINDOW_SEC", 300),
		IdemTTL:      durSec("HOOKRELAY_IDEM_TTL_SEC", 86400),
		Workers:      envInt("HOOKRELAY_WORKERS", 4),
		PublicBase:   strings.TrimRight(env("HOOKRELAY_PUBLIC_BASE", "http://127.0.0.1:8080"), "/"),
		LoopbackPath: "/api/v1/loopback",
	}
	if c.IngestSecret == "" {
		return c, fmt.Errorf("HOOKRELAY_INGEST_SECRET is empty")
	}
	if c.Workers < 1 {
		c.Workers = 1
	}
	if c.Workers > 32 {
		c.Workers = 32
	}
	if !strings.HasPrefix(c.Addr, ":") && !strings.Contains(c.Addr, ":") {
		c.Addr = ":" + c.Addr
	}
	return c, nil
}

func env(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func durSec(key string, def int) time.Duration {
	n := envInt(key, def)
	if n < 1 {
		n = def
	}
	return time.Duration(n) * time.Second
}
