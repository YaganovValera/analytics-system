// api-gateway/cmd/api-gateway/main.go
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
	"github.com/YaganovValera/analytics-system/services/api-gateway/internal/app"
	"github.com/YaganovValera/analytics-system/services/api-gateway/internal/config"
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

	log, err := logger.New(cfg.Logging)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger init error: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	log.Info("starting api-gateway",
		zap.String("service.name", cfg.ServiceName),
		zap.String("service.version", cfg.ServiceVersion),
		zap.String("config.path", configPath),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx, cfg, log); err != nil {
		if errors.Is(err, context.Canceled) {
			log.Info("api-gateway: shutdown complete")
		} else {
			log.Error("api-gateway exited with error", zap.Error(err))
			os.Exit(1)
		}
	}

	log.Info("api-gateway shut down cleanly")
}
