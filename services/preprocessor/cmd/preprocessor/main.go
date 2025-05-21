// preprocessor/cmd/preprocessor/main.go
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
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/app"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/config"
)

func main() {
	// 0. Command-line flags
	var configPath string
	pflag.StringVar(&configPath, "config", "config/config.yaml", "path to config file")
	pflag.Parse()

	// 1. Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load error: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize logger
	log, err := logger.New(logger.Config{
		Level:   cfg.Logging.Level,
		DevMode: cfg.Logging.DevMode,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger init error: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	log.Info("starting preprocessor service",
		zap.String("service.name", cfg.ServiceName),
		zap.String("service.version", cfg.ServiceVersion),
		zap.String("config.path", configPath),
	)

	// 3. Build context that cancels on SIGINT/SIGTERM
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 4. Run application
	if err := app.Run(ctx, cfg, log); err != nil {
		if errors.Is(err, context.Canceled) {
			log.Info("application shutdown complete")
		} else {
			log.Error("application exited with error", zap.Error(err))
			os.Exit(1)
		}
	}

	log.Info("shutdown complete")
}
