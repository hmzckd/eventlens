package config

import (
	"log/slog"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(key string) string {
		if key == "EVENTLENS_WRITE_KEY" {
			if v, ok := values[key]; ok {
				return v
			}
			return "test-write-key"
		}
		return values[key]
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want info", cfg.LogLevel)
	}
	if cfg.ClickHouseAddr != "localhost:9000" || cfg.ClickHouseDatabase != "eventlens" {
		t.Errorf("ClickHouse = %s/%s, want localhost:9000/eventlens", cfg.ClickHouseAddr, cfg.ClickHouseDatabase)
	}
	if cfg.QueueCapacity != 50_000 || cfg.BatchSize != 10_000 || cfg.FlushInterval != time.Second {
		t.Errorf("pipeline = %d/%d/%s, want 50000/10000/1s", cfg.QueueCapacity, cfg.BatchSize, cfg.FlushInterval)
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"EVENTLENS_ADDR":      "127.0.0.1:9090",
		"EVENTLENS_LOG_LEVEL": "debug",

		"EVENTLENS_CLICKHOUSE_ADDR":     "ch:9440",
		"EVENTLENS_CLICKHOUSE_DATABASE": "analytics",
		"EVENTLENS_CLICKHOUSE_USER":     "reader",
		"EVENTLENS_CLICKHOUSE_PASSWORD": "secret",

		"EVENTLENS_QUEUE_CAPACITY": "2000",
		"EVENTLENS_BATCH_SIZE":     "1000",
		"EVENTLENS_FLUSH_INTERVAL": "250ms",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != "127.0.0.1:9090" {
		t.Errorf("Addr = %q", cfg.Addr)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want debug", cfg.LogLevel)
	}
	if cfg.ClickHouseAddr != "ch:9440" || cfg.ClickHouseDatabase != "analytics" ||
		cfg.ClickHouseUser != "reader" || cfg.ClickHousePassword != "secret" {
		t.Errorf("ClickHouse settings not read: %+v", cfg)
	}
	if cfg.QueueCapacity != 2000 || cfg.BatchSize != 1000 || cfg.FlushInterval != 250*time.Millisecond {
		t.Errorf("pipeline settings not read: %+v", cfg)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	cases := map[string]map[string]string{
		"addr without port":           {"EVENTLENS_ADDR": "localhost"},
		"unknown log level":           {"EVENTLENS_LOG_LEVEL": "loud"},
		"clickhouse no port":          {"EVENTLENS_CLICKHOUSE_ADDR": "clickhouse"},
		"missing write key":           {"EVENTLENS_WRITE_KEY": ""},
		"capacity not a number":       {"EVENTLENS_QUEUE_CAPACITY": "lots"},
		"capacity below one request":  {"EVENTLENS_QUEUE_CAPACITY": "499", "EVENTLENS_BATCH_SIZE": "100"},
		"batch larger than the queue": {"EVENTLENS_QUEUE_CAPACITY": "1000", "EVENTLENS_BATCH_SIZE": "2000"},
		"zero flush interval":         {"EVENTLENS_FLUSH_INTERVAL": "0s"},
	}
	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(env(values)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
