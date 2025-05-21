// auth/cmd/auth/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/services/auth/internal/app"
	"github.com/YaganovValera/analytics-system/services/auth/internal/config"
)

func main() {
	// --- CLI flags
	var configPath string
	pflag.StringVar(&configPath, "config", "config/config.yaml", "path to config file")
	pflag.Parse()

	// --- Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load error: %v\n", err)
		os.Exit(1)
	}

	// --- Init logger
	log, err := logger.New(logger.Config{
		Level:   cfg.Logging.Level,
		DevMode: cfg.Logging.DevMode,
		Format:  cfg.Logging.Format,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger init error: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	log.Info("starting auth service",
		zap.String("service.name", cfg.ServiceName),
		zap.String("service.version", cfg.ServiceVersion),
		zap.String("config.path", configPath),
	)

	// --- Graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --- Run app
	if err := app.Run(ctx, cfg, log); err != nil {
		if errors.Is(err, context.Canceled) {
			log.Info("auth: shutdown complete")
		} else {
			log.Error("auth exited with error", zap.Error(err))
			os.Exit(1)
		}
	}

	log.Info("auth shutdown cleanly")
}
