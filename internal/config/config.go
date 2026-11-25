package config

import (
	"fmt"
	"log/slog"
	"net"
)

type Config struct {
	Addr     string
	LogLevel slog.Level

	ClickHouseAddr     string
	ClickHouseDatabase string
	ClickHouseUser     string
	ClickHousePassword string

	WriteKey string
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Addr:               ":8080",
		LogLevel:           slog.LevelInfo,
		ClickHouseAddr:     "localhost:9000",
		ClickHouseDatabase: "eventlens",
		ClickHouseUser:     "eventlens",
		ClickHousePassword: "eventlens",
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

	return cfg, nil
}
