// github.com/YaganovValera/analytics-system/services/market-data-collector/internal/app/collector.go
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common"
	httpserver "github.com/YaganovValera/analytics-system/common/httpserver"
	producer "github.com/YaganovValera/analytics-system/common/kafka/producer"
	"github.com/YaganovValera/analytics-system/common/logger"
	"github.com/YaganovValera/analytics-system/common/telemetry"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/config"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/processor"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func Run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	common.InitServiceName(cfg.ServiceName)
	metrics.Register(nil)

	shutdownTracer, err := telemetry.InitTracer(ctx, telemetry.Config{
		Endpoint:        cfg.Telemetry.OTLPEndpoint,
		ServiceName:     cfg.ServiceName,
		ServiceVersion:  cfg.ServiceVersion,
		Insecure:        cfg.Telemetry.Insecure,
		SamplerRatio:    1.0,
		ReconnectPeriod: 5 * time.Second,
	}, log)
	if err != nil {
		return fmt.Errorf("init tracer: %w", err)
	}
	defer shutdownSafe(ctx, "telemetry", shutdownTracer, log)

	wsConn, err := binance.NewConnector(binance.Config{
		URL:              cfg.Binance.WSURL,
		Streams:          cfg.Binance.Symbols,
		ReadTimeout:      cfg.Binance.ReadTimeout,
		SubscribeTimeout: cfg.Binance.SubscribeTimeout,
		BackoffConfig:    cfg.Binance.Backoff,
		BufferSize:       cfg.Binance.BufferSize,
	}, log)
	if err != nil {
		return fmt.Errorf("binance: %w", err)
	}
	defer shutdownSafe(ctx, "ws-connector", func(ctx context.Context) error {
		return wsConn.Close()
	}, log)

	prod, err := producer.New(ctx, producer.Config{
		Brokers:        cfg.Kafka.Brokers,
		RequiredAcks:   cfg.Kafka.Acks,
		Timeout:        cfg.Kafka.Timeout,
		Compression:    cfg.Kafka.Compression,
		FlushFrequency: cfg.Kafka.FlushFrequency,
		FlushMessages:  cfg.Kafka.FlushMessages,
		Backoff:        cfg.Kafka.Backoff,
	}, log)
	if err != nil {
		return fmt.Errorf("kafka: %w", err)
	}
	defer shutdownSafe(ctx, "kafka-producer", func(ctx context.Context) error {
		return prod.Close()
	}, log)

	// два отдельных процессора
	tradeProc := processor.NewTradeProcessor(prod, cfg.Kafka.RawTopic, log)
	depthProc := processor.NewDepthProcessor(prod, cfg.Kafka.OrderBookTopic, log)

	readiness := func() error { return prod.Ping(ctx) }
	httpSrv, err := httpserver.New(httpserver.Config{
		Addr:            fmt.Sprintf(":%d", cfg.HTTP.Port),
		ReadTimeout:     cfg.HTTP.ReadTimeout,
		WriteTimeout:    cfg.HTTP.WriteTimeout,
		IdleTimeout:     cfg.HTTP.IdleTimeout,
		ShutdownTimeout: cfg.HTTP.ShutdownTimeout,
		MetricsPath:     cfg.HTTP.MetricsPath,
		HealthzPath:     cfg.HTTP.HealthzPath,
		ReadyzPath:      cfg.HTTP.ReadyzPath,
	}, readiness, log, nil)
	if err != nil {
		return fmt.Errorf("httpserver: %w", err)
	}

	log.WithContext(ctx).Info("collector: components initialized, starting loops")
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return httpSrv.Run(ctx)
	})

	g.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			msgCh, err := wsConn.Stream(ctx)
			if err != nil {
				log.WithContext(ctx).Error("ws stream error, retrying", zap.Error(err))
				time.Sleep(1 * time.Second)
				continue
			}

		inner:
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case raw, ok := <-msgCh:
					if !ok {
						log.WithContext(ctx).Warn("ws channel closed, restarting")
						break inner
					}

					var err error
					switch raw.Type {
					case processor.EventTypeTrade:
						err = tradeProc.Process(ctx, raw)
					case processor.EventTypeDepth:
						err = depthProc.Process(ctx, raw)
					default:
						log.WithContext(ctx).Debug("unknown event type", zap.String("type", raw.Type))
					}
					if err != nil {
						log.WithContext(ctx).Error("processor error", zap.Error(err))
					}
				}
			}

			time.Sleep(1 * time.Second)
		}
	})

	if err := g.Wait(); err != nil {
		if errors.Is(err, context.Canceled) {
			log.WithContext(ctx).Info("collector exited on context cancel")
			return nil
		}
		log.WithContext(ctx).Error("collector exited with error", zap.Error(err))
		return err
	}
	log.WithContext(ctx).Info("collector exited cleanly")
	return nil

}

func shutdownSafe(ctx context.Context, name string, fn func(context.Context) error, log *logger.Logger) {
	log.WithContext(ctx).Info(name + ": shutting down")
	if err := fn(ctx); err != nil {
		log.WithContext(ctx).Error(name+" shutdown failed", zap.Error(err))
	} else {
		log.WithContext(ctx).Info(name + ": shutdown complete")
	}
}
