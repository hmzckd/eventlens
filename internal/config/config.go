package config

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"
)

const maxEventsPerRequest = 500

type Config struct {
	Addr     string
	LogLevel slog.Level

	ClickHouseAddr     string
	ClickHouseDatabase string
	ClickHouseUser     string
	ClickHousePassword string

	WriteKey string

	QueueCapacity int
	BatchSize     int
	FlushInterval time.Duration
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Addr:               ":8080",
		LogLevel:           slog.LevelInfo,
		ClickHouseAddr:     "localhost:9000",
		ClickHouseDatabase: "eventlens",
		ClickHouseUser:     "eventlens",
		ClickHousePassword: "eventlens",
		QueueCapacity:      50_000,
		BatchSize:          10_000,
		FlushInterval:      time.Second,
	}

	if v := getenv("EVENTLENS_ADDR"); v != "" {
		cfg.Addr = v
	}
	if _, _, err := net.SplitHostPort(cfg.Addr); err != nil {
		return Config{}, fmt.Errorf("EVENTLENS_ADDR must look like host:port or :port, got %q", cfg.Addr)
	}

	if v := getenv("EVENTLENS_LOG_LEVEL"); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return Config{}, fmt.Errorf("EVENTLENS_LOG_LEVEL must be debug, info, warn or error, got %q", v)
		}
	}

	if v := getenv("EVENTLENS_CLICKHOUSE_ADDR"); v != "" {
		cfg.ClickHouseAddr = v
	}
	if _, _, err := net.SplitHostPort(cfg.ClickHouseAddr); err != nil {
		return Config{}, fmt.Errorf("EVENTLENS_CLICKHOUSE_ADDR must look like host:port, got %q", cfg.ClickHouseAddr)
	}
	if v := getenv("EVENTLENS_CLICKHOUSE_DATABASE"); v != "" {
		cfg.ClickHouseDatabase = v
	}
	if v := getenv("EVENTLENS_CLICKHOUSE_USER"); v != "" {
		cfg.ClickHouseUser = v
	}
	if v := getenv("EVENTLENS_CLICKHOUSE_PASSWORD"); v != "" {
		cfg.ClickHousePassword = v
	}

	cfg.WriteKey = getenv("EVENTLENS_WRITE_KEY")
	if cfg.WriteKey == "" {
		return Config{}, fmt.Errorf("EVENTLENS_WRITE_KEY is required: clients send it in the X-Write-Key header")
	}

	var err error
	if cfg.QueueCapacity, err = intVar(getenv, "EVENTLENS_QUEUE_CAPACITY", cfg.QueueCapacity); err != nil {
		return Config{}, err
	}
	if cfg.BatchSize, err = intVar(getenv, "EVENTLENS_BATCH_SIZE", cfg.BatchSize); err != nil {
		return Config{}, err
	}
	if v := getenv("EVENTLENS_FLUSH_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("EVENTLENS_FLUSH_INTERVAL must be a positive duration such as 1s or 500ms, got %q", v)
		}
		cfg.FlushInterval = d
	}
	if cfg.QueueCapacity < maxEventsPerRequest {
		return Config{}, fmt.Errorf("EVENTLENS_QUEUE_CAPACITY must be at least %d (one full request), got %d", maxEventsPerRequest, cfg.QueueCapacity)
	}
	if cfg.BatchSize > cfg.QueueCapacity {
		return Config{}, fmt.Errorf("EVENTLENS_BATCH_SIZE (%d) must not be larger than EVENTLENS_QUEUE_CAPACITY (%d)", cfg.BatchSize, cfg.QueueCapacity)
	}

	return cfg, nil
}

func intVar(getenv func(string) string, key string, def int) (int, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive whole number, got %q", key, v)
	}
	return n, nil
}
