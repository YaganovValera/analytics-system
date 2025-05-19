// github.com/YaganovValera/analytics-system/services/market-data-collector/internal/app/collector.go
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/backoff"
	httpserver "github.com/YaganovValera/analytics-system/common/httpserver"
	producer "github.com/YaganovValera/analytics-system/common/kafka/producer"
	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/common/serviceid"
	"github.com/YaganovValera/analytics-system/common/telemetry"

	// pkg/binance — здесь лежит Config и NewConnector
	pkgBinance "github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance"
	// internal/transport/binance — только для StreamWithMetrics и метрик
	transportBinance "github.com/YaganovValera/analytics-system/services/market-data-collector/internal/transport/binance"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/config"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/processor"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func Run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	serviceid.InitServiceName(cfg.ServiceName)
	metrics.Register(nil)
	transportBinance.RegisterMetrics(nil)

	// Инициализируем трассировку
	cfg.Telemetry.ServiceName = cfg.ServiceName
	cfg.Telemetry.ServiceVersion = cfg.ServiceVersion
	shutdownTracer, err := telemetry.InitTracer(ctx, cfg.Telemetry, log)
	if err != nil {
		return fmt.Errorf("init tracer: %w", err)
	}
	defer shutdownSafe(ctx, "telemetry", func() error { return shutdownTracer(ctx) }, log)

	// 1) Создаём pkg/binance connector с правильным Config
	binanceCfg := pkgBinance.Config{
		URL:              cfg.Binance.URL,
		Streams:          cfg.Binance.Streams,
		BufferSize:       cfg.Binance.BufferSize,
		ReadTimeout:      cfg.Binance.ReadTimeout,
		SubscribeTimeout: cfg.Binance.SubscribeTimeout,
		BackoffConfig:    cfg.Binance.BackoffConfig,
	}
	wsConn, err := pkgBinance.NewConnector(binanceCfg, log)
	if err != nil {
		return fmt.Errorf("binance connector init: %w", err)
	}
	defer shutdownSafe(ctx, "ws-connector", wsConn.Close, log)

	// 2) Kafka Producer
	kafkaProd, err := producer.New(ctx, producer.Config{
		Brokers:        cfg.Kafka.Brokers,
		RequiredAcks:   cfg.Kafka.RequiredAcks,
		Timeout:        cfg.Kafka.Timeout,
		Compression:    cfg.Kafka.Compression,
		FlushFrequency: cfg.Kafka.FlushFrequency,
		FlushMessages:  cfg.Kafka.FlushMessages,
		Backoff:        cfg.Kafka.Backoff,
	}, log)
	if err != nil {
		return fmt.Errorf("kafka producer init: %w", err)
	}
	defer shutdownSafe(ctx, "kafka-producer", kafkaProd.Close, log)

	tradeProc := processor.NewTradeProcessor(kafkaProd, cfg.Kafka.RawTopic, log)
	depthProc := processor.NewDepthProcessor(kafkaProd, cfg.Kafka.OrderBookTopic, log)

	// HTTP-сервер
	readiness := func() error { return kafkaProd.Ping(ctx) }
	httpSrv, err := httpserver.New(
		httpserver.Config{
			Port:            cfg.HTTP.Port,
			ReadTimeout:     cfg.HTTP.ReadTimeout,
			WriteTimeout:    cfg.HTTP.WriteTimeout,
			IdleTimeout:     cfg.HTTP.IdleTimeout,
			ShutdownTimeout: cfg.HTTP.ShutdownTimeout,
			MetricsPath:     cfg.HTTP.MetricsPath,
			HealthzPath:     cfg.HTTP.HealthzPath,
			ReadyzPath:      cfg.HTTP.ReadyzPath,
		},
		readiness,
		log,
		nil,
		httpserver.RecoverMiddleware,
		httpserver.CORSMiddleware(),
	)
	if err != nil {
		return fmt.Errorf("httpserver init: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)
	// HTTP
	g.Go(func() error { return httpSrv.Run(ctx) })

	// Основной WS→Kafka цикл
	g.Go(func() error {
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			// 3) Подключаемся через backoff и StreamWithMetrics
			var msgCh <-chan pkgBinance.RawMessage
			if err := backoff.Execute(ctx, cfg.Binance.BackoffConfig,
				func(ctx context.Context) error {
					ch, e := transportBinance.StreamWithMetrics(ctx, wsConn)
					if e == nil {
						msgCh = ch
					}
					return e
				},
				func(ctx context.Context, e error, delay time.Duration, attempt int) {
					log.WithContext(ctx).Warn("ws reconnect retry",
						zap.Error(e),
						zap.Duration("delay", delay),
						zap.Int("attempt", attempt),
					)
				},
			); err != nil {
				return fmt.Errorf("ws connect failed: %w", err)
			}

			// 4) Обработка через dispatcher (trade + depth)
			if err := processor.DispatchStream(ctx, msgCh, tradeProc, log.Sugar().Desugar()); err != nil {
				log.WithContext(ctx).Error("dispatch trade", zap.Error(err))
			}
			if err := processor.DispatchStream(ctx, msgCh, depthProc, log.Sugar().Desugar()); err != nil {
				log.WithContext(ctx).Error("dispatch depth", zap.Error(err))
			}

			// loop continues on channel close
		}
	})

	if err := g.Wait(); err != nil {
		if errors.Is(err, context.Canceled) {
			log.WithContext(ctx).Info("collector stopped by context")
			return nil
		}
		return err
	}
	return nil
}

// shutdownSafe оборачивает вызов Close()/Shutdown() с логированием
func shutdownSafe(ctx context.Context, name string, fn func() error, log *logger.Logger) {
	log.WithContext(ctx).Info(fmt.Sprintf("%s: shutting down", name))
	if err := fn(); err != nil {
		log.WithContext(ctx).Error(fmt.Sprintf("%s shutdown error", name), zap.Error(err))
	} else {
		log.WithContext(ctx).Info(fmt.Sprintf("%s: shutdown complete", name))
	}
}
