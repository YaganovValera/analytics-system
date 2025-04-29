package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	pflag "github.com/spf13/pflag"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/app"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/config"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"
	pkgtelemetry "github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/telemetry"
)

func main() {
	// Флаг --config
	configPath := flag.String("config", "config/config.yaml", "path to config file")
	pflag.Parse()

	// 1. Загрузить конфиг
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	if err := cfg.Print(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to print config: %v\n", err)
	}

	// 2. Инициализация логгера
	log, err := logger.New(cfg.Logging.Level, cfg.Logging.DevMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger init error: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	// 3. Контекст с отменой по сигналам
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 4. Инициализация OpenTelemetry
	shutdownTracer, err := pkgtelemetry.InitTracer(
		ctx,
		cfg.Telemetry.OTLPEndpoint,
		cfg.ServiceName,
		cfg.ServiceVersion,
		cfg.Telemetry.Insecure,
		log,
	)
	if err != nil {
		log.Sugar().Fatalw("telemetry init error", "error", err)
	}
	defer func() {
		if err := shutdownTracer(ctx); err != nil {
			log.Sugar().Errorw("tracer shutdown error", "error", err)
		}
	}()

	log.Sugar().Infow("starting service",
		"service.name", cfg.ServiceName,
		"service.version", cfg.ServiceVersion,
	)

	// 5. Запуск основного приложения
	if err := app.Run(ctx, cfg, log); err != nil {
		log.Sugar().Errorw("application exited with error", "error", err)
		os.Exit(1)
	}

	log.Sugar().Infow("shutdown complete")
}
