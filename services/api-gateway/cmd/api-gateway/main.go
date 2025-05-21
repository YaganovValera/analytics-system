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
	// CLI флаг: путь до конфига
	var configPath string
	pflag.StringVar(&configPath, "config", "config/config.yaml", "Path to configuration file")
	pflag.Parse()

	// Загрузка конфигурации
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Инициализация логгера
	log, err := logger.New(cfg.Logging)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	log.Info("starting API Gateway",
		zap.String("service", cfg.ServiceName),
		zap.String("version", cfg.ServiceVersion),
		zap.String("config", configPath),
	)

	// Контекст с отменой по сигналу SIGINT/SIGTERM
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Запуск приложения
	if err := app.Run(ctx, cfg, log); err != nil {
		if errors.Is(err, context.Canceled) {
			log.Info("shutdown complete")
		} else {
			log.Error("gateway exited with error", zap.Error(err))
			os.Exit(1)
		}
	}

	log.Info("gateway exited cleanly")
}
