package app

import (
	"log/slog"

	"github.com/spiehdid/crypto-price-aggregator/internal/config"
)

type LogLevelSetter interface {
	SetLevel(level slog.Level)
}

type Reloader struct {
	configPath string
	levelVar   *slog.LevelVar
}

func NewReloader(configPath string, levelVar *slog.LevelVar) *Reloader {
	return &Reloader{
		configPath: configPath,
		levelVar:   levelVar,
	}
}

func (r *Reloader) Reload() error {
	slog.Info("reloading configuration...")

	cfg, err := config.Load(r.configPath)
	if err != nil {
		slog.Error("config reload failed", "error", err)
		return err
	}

	if r.levelVar != nil {
		var level slog.Level
		switch cfg.Telemetry.Logs.Level {
		case "debug":
			level = slog.LevelDebug
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		default:
			level = slog.LevelInfo
		}
		r.levelVar.Set(level)
		slog.Info("log level updated", "level", cfg.Telemetry.Logs.Level)
	}

	slog.Info("configuration reloaded successfully")
	return nil
}
