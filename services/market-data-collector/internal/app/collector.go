// market-data-collector/internal/app/collector.go
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

	transportBinance "github.com/YaganovValera/analytics-system/services/market-data-collector/internal/transport/binance"
	pkgBinance "github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance"

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

	// === Telemetry ===
	cfg.Telemetry.ServiceName = cfg.ServiceName
	cfg.Telemetry.ServiceVersion = cfg.ServiceVersion
	shutdownTracer, err := telemetry.InitTracer(ctx, cfg.Telemetry, log)
	if err != nil {
		return fmt.Errorf("init tracer: %w", err)
	}
	defer shutdownSafe(ctx, "telemetry", func() error { return shutdownTracer(ctx) }, log)

	// === Binance Connector
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

	wsManager := transportBinance.NewWSManager(wsConn)
	defer shutdownSafe(ctx, "ws-manager", func() error {
		wsManager.Stop()
		return nil
	}, log)

	// === Kafka Producer
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

	// === Processors
	tradeProc := processor.NewTradeProcessor(kafkaProd, cfg.Kafka.RawTopic, log)
	depthProc := processor.NewDepthProcessor(kafkaProd, cfg.Kafka.OrderBookTopic, log)

	// === HTTP Server
	readiness := func() error { return kafkaProd.Ping(ctx) }
	httpSrv, err := httpserver.New(
		cfg.HTTP,
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

	// HTTP server
	g.Go(func() error { return httpSrv.Run(ctx) })

	// WebSocket loop
	g.Go(func() error {
		for {
			if ctx.Err() != nil {
				wsManager.Stop()
				return ctx.Err()
			}

			var msgCh <-chan pkgBinance.RawMessage
			var cancel context.CancelFunc

			err := backoff.Execute(ctx, cfg.Binance.BackoffConfig,
				func(ctx context.Context) error {
					ch, cancelFn, err := wsManager.Start(ctx)
					if err == nil {
						msgCh = ch
						cancel = cancelFn
					}
					return err
				},
				func(ctx context.Context, err error, delay time.Duration, attempt int) {
					log.WithContext(ctx).Warn("ws reconnect retry",
						zap.Error(err),
						zap.Duration("delay", delay),
						zap.Int("attempt", attempt),
					)
				},
			)
			if err != nil {
				wsManager.Stop()
				return fmt.Errorf("ws connect failed: %w", err)
			}

			router := processor.NewRouter(log.Named("processor"))
			router.Register("trade", tradeProc)
			router.Register("depthUpdate", depthProc)

			if err := router.Run(ctx, msgCh); err != nil {
				log.WithContext(ctx).Error("router exited", zap.Error(err))
			}

			if cancel != nil {
				cancel()
			}
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

func shutdownSafe(ctx context.Context, name string, fn func() error, log *logger.Logger) {
	log.WithContext(ctx).Info(fmt.Sprintf("%s: shutting down", name))
	if err := fn(); err != nil {
		log.WithContext(ctx).Error(fmt.Sprintf("%s shutdown error", name), zap.Error(err))
	} else {
		log.WithContext(ctx).Info(fmt.Sprintf("%s: shutdown complete", name))
	}
}
