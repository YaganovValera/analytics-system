// services/analytics-api/cmd/analytics-api/main.go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/app"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/config"
)

func main() {
	// 1) Путь до конфига
	var configPath string
	pflag.StringVar(&configPath, "config", "config/config.yaml", "path to config file")
	pflag.Parse()

	// 2) Загрузка конфига
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load error: %v\n", err)
		os.Exit(1)
	}

	// 3) Логгер
	log, err := logger.New(logger.Config{
		Level:   cfg.Logging.Level,
		DevMode: cfg.Logging.DevMode,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger init error: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	// 3a) При dev-режиме — вывести конфиг
	if cfg.Logging.DevMode {
		cfg.Print()
	}

	log.Info("starting analytics-api service",
		zap.String("service.name", cfg.ServiceName),
		zap.String("service.version", cfg.ServiceVersion),
		zap.String("config.path", configPath),
	)

	// 4) Контекст с отменой по SIGINT/SIGTERM
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 5) Запуск приложения
	if err := app.Run(ctx, cfg, log); err != nil {
		log.Error("application exited with error", zap.Error(err))
		os.Exit(1)
	}

	log.Info("shutdown complete")
}
