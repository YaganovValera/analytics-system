// services/market-data-collector/internal/app/collector.go
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/config"
	httpserver "github.com/YaganovValera/analytics-system/services/market-data-collector/internal/http"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/processor"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/kafka"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/telemetry"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Run wires up and runs the collector service.
func Run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	// 1) Регистрация метрик
	metrics.Register(nil)

	// 2) Инициализация OpenTelemetry
	shutdownOTel, err := telemetry.InitTracer(ctx, telemetry.Config{
		Endpoint:       cfg.Telemetry.OTLPEndpoint,
		ServiceName:    cfg.ServiceName,
		ServiceVersion: cfg.ServiceVersion,
		Insecure:       cfg.Telemetry.Insecure,
	}, log)
	if err != nil {
		return fmt.Errorf("telemetry init: %w", err)
	}
	// defer shutdown OTEL last
	defer func() {
		log.Info("collector: shutting down telemetry")
		_ = shutdownOTel(context.Background())
	}()

	// 3) Binance WS connector
	wsConn, err := binance.NewConnector(binance.Config{
		URL:              cfg.Binance.WSURL,
		Streams:          cfg.Binance.Symbols,
		ReadTimeout:      cfg.Binance.ReadTimeout,
		SubscribeTimeout: cfg.Binance.SubscribeTimeout,
		BackoffConfig:    cfg.Binance.Backoff,
	}, log)
	if err != nil {
		return fmt.Errorf("binance connector init: %w", err)
	}
	// defer WS close before OTEL shutdown
	defer func() {
		if err := wsConn.Close(); err != nil {
			log.WithContext(ctx).Error("collector: ws close failed", zap.Error(err))
		} else {
			log.WithContext(ctx).Info("collector: ws connection closed")
		}
	}()

	// 4) Kafka producer
	prod, err := kafka.NewProducer(ctx, kafka.Config{
		Brokers:        cfg.Kafka.Brokers,
		RequiredAcks:   cfg.Kafka.Acks,
		Timeout:        cfg.Kafka.Timeout,
		Compression:    cfg.Kafka.Compression,
		FlushFrequency: cfg.Kafka.FlushFrequency,
		FlushMessages:  cfg.Kafka.FlushMessages,
		Backoff:        cfg.Kafka.Backoff,
	}, log)
	if err != nil {
		return fmt.Errorf("kafka producer init: %w", err)
	}
	// defer producer close before WS close
	defer func() {
		if err := prod.Close(); err != nil {
			log.WithContext(ctx).Error("collector: producer close failed", zap.Error(err))
		} else {
			log.WithContext(ctx).Info("collector: kafka producer closed")
		}
	}()

	// 5) Processor pipeline
	proc := processor.New(
		prod,
		processor.Topics{
			RawTopic:       cfg.Kafka.RawTopic,
			OrderBookTopic: cfg.Kafka.OrderBookTopic,
		},
		log,
	)

	// 6) HTTP endpoints: /metrics, /healthz, /readyz
	readiness := func() error {
		if err := prod.Ping(); err != nil {
			return fmt.Errorf("kafka not ready: %w", err)
		}
		return nil
	}
	httpSrv := httpserver.NewServer(
		fmt.Sprintf(":%d", cfg.HTTP.Port),
		readiness,
		log,
	)

	log.WithContext(ctx).Info("collector: all components initialized, starting loops")

	// 7) Run HTTP server and self-healing WS→Processor loop concurrently
	g, ctx := errgroup.WithContext(ctx)

	// HTTP server goroutine
	g.Go(func() error {
		if err := httpSrv.Start(ctx); err != nil {
			log.WithContext(ctx).Error("collector: HTTP server stopped with error", zap.Error(err))
			return err
		}
		return nil
	})

	// WS→Processor loop with self-healing and backpressure
	g.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			msgCh, err := wsConn.Stream(ctx)
			if err != nil {
				log.WithContext(ctx).Error("collector: ws stream error, retrying", zap.Error(err))
				time.Sleep(1 * time.Second)
				continue
			}

			// метка для выхода из внутреннего цикла
		readLoop:
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case raw, ok := <-msgCh:
					if !ok {
						log.WithContext(ctx).Warn("collector: ws channel closed, restarting")
						// выходим из внутреннего for, чтобы восстановить стрим
						break readLoop
					}
					if err := proc.Process(ctx, raw); err != nil {
						log.WithContext(ctx).Error("collector: processor error", zap.Error(err))
					}
				}
			}

			// небольшая задержка перед повторным подключением
			time.Sleep(1 * time.Second)
		}
	})

	// 8) Wait for either loop to exit
	if err := g.Wait(); err != nil {
		log.WithContext(ctx).Error("collector: exiting with error", zap.Error(err))
	} else {
		log.WithContext(ctx).Info("collector: exiting normally")
	}
	return g.Wait()
}
