package app

import (
	"context"
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

// Run wires up and runs the collector service.
func Run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	// -------------------------------------------------------------------------
	// 0) Сквозной service-label для всех подсистем
	// -------------------------------------------------------------------------
	common.InitServiceName(cfg.ServiceName)
	binance.SetServiceLabel(cfg.ServiceName)

	// -------------------------------------------------------------------------
	// 1) Внутренние метрики
	// -------------------------------------------------------------------------
	metrics.Register(nil)

	// -------------------------------------------------------------------------
	// 2) OpenTelemetry
	// -------------------------------------------------------------------------
	shutdownOTel, err := telemetry.InitTracer(ctx, telemetry.Config{
		Endpoint:       cfg.Telemetry.OTLPEndpoint,
		ServiceName:    cfg.ServiceName,
		ServiceVersion: cfg.ServiceVersion,
		Insecure:       cfg.Telemetry.Insecure,
	}, log)
	if err != nil {
		return fmt.Errorf("telemetry init: %w", err)
	}
	defer func() {
		log.Info("collector: shutting down telemetry")
		_ = shutdownOTel(context.Background())
	}()

	// -------------------------------------------------------------------------
	// 3) Binance WS-коннектор
	// -------------------------------------------------------------------------
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
	defer func() {
		if err := wsConn.Close(); err != nil {
			log.WithContext(ctx).Error("ws close failed", zap.Error(err))
		} else {
			log.WithContext(ctx).Info("ws connection closed")
		}
	}()

	// -------------------------------------------------------------------------
	// 4) Kafka Producer
	// -------------------------------------------------------------------------
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
		return fmt.Errorf("kafka producer init: %w", err)
	}
	defer func() {
		if err := prod.Close(); err != nil {
			log.WithContext(ctx).Error("producer close failed", zap.Error(err))
		} else {
			log.WithContext(ctx).Info("kafka producer closed")
		}
	}()

	// -------------------------------------------------------------------------
	// 5) Processor
	// -------------------------------------------------------------------------
	proc := processor.New(
		prod,
		processor.Topics{
			RawTopic:       cfg.Kafka.RawTopic,
			OrderBookTopic: cfg.Kafka.OrderBookTopic,
		},
		log,
	)

	// -------------------------------------------------------------------------
	// 6) HTTP / Metrics server
	// -------------------------------------------------------------------------
	readiness := func() error { return prod.Ping(ctx) }

	httpSrv, err := httpserver.New(
		httpserver.Config{
			Addr:            fmt.Sprintf(":%d", cfg.HTTP.Port),
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
	)
	if err != nil {
		return fmt.Errorf("http server init: %w", err)
	}

	log.WithContext(ctx).Info("collector: components initialized, starting loops")

	// -------------------------------------------------------------------------
	// 7) Run HTTP + WS→Processor loops
	// -------------------------------------------------------------------------
	g, ctx := errgroup.WithContext(ctx)

	// HTTP-server loop (блокирует внутри Start)
	g.Go(func() error {
		return httpSrv.Start(ctx)
	})

	// WebSocket ingestion & processing loop
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
					if err := proc.Process(ctx, raw); err != nil {
						log.WithContext(ctx).Error("processor error", zap.Error(err))
					}
				}
			}

			time.Sleep(1 * time.Second)
		}
	})

	// -------------------------------------------------------------------------
	// 8) Wait & exit
	// -------------------------------------------------------------------------
	err = g.Wait()
	if err != nil {
		log.WithContext(ctx).Error("collector: exiting with error", zap.Error(err))
	} else {
		log.WithContext(ctx).Info("collector: exiting normally")
	}
	return err
}
