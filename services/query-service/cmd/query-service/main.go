// query-service/cmd/query-service/main.go
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
	"github.com/YaganovValera/analytics-system/services/query-service/internal/app"
	"github.com/YaganovValera/analytics-system/services/query-service/internal/config"
)

func main() {
	var configPath string
	pflag.StringVar(&configPath, "config", "config/config.yaml", "path to config file")
	pflag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load error: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.New(logger.Config{
		Level:   cfg.Logging.Level,
		DevMode: cfg.Logging.DevMode,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger init error: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	log.Info("starting query-service",
		zap.String("service.name", cfg.ServiceName),
		zap.String("service.version", cfg.ServiceVersion),
		zap.String("config.path", configPath),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx, cfg, log); err != nil {
		if errors.Is(err, context.Canceled) {
			log.Info("query-service shutdown complete")
		} else {
			log.Error("query-service exited with error", zap.Error(err))
			os.Exit(1)
		}
	}

	log.Info("shutdown complete")
}
