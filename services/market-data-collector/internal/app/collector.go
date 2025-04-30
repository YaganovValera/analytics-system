// services/market-data-collector/internal/app/collector.go
package app

import (
	"context"
	"fmt"

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
	// 1) Debug: print loaded config
	cfg.Print()

	// 2) Processor metrics
	metrics.Register(nil)

	// 3) OpenTelemetry
	shutdownOTel, err := telemetry.InitTracer(ctx, telemetry.Config{
		Endpoint:       cfg.Telemetry.OTLPEndpoint,
		ServiceName:    cfg.ServiceName,
		ServiceVersion: cfg.ServiceVersion,
		Insecure:       cfg.Telemetry.Insecure,
	}, log)
	if err != nil {
		return fmt.Errorf("telemetry init: %w", err)
	}
	defer func() { _ = shutdownOTel(context.Background()) }()

	// 4) HTTP endpoints: /metrics, /healthz, /readyz
	httpSrv := httpserver.NewServer(
		fmt.Sprintf(":%d", cfg.HTTP.Port),
		func() error { return nil }, // placeholder readiness
		log,
	)

	// 5) Binance WS connector
	wsConn, err := binance.NewConnector(binance.Config{
		URL:           cfg.Binance.WSURL,
		Streams:       cfg.Binance.Symbols,
		ReadTimeout:   cfg.Binance.ReadTimeout,
		BackoffConfig: cfg.Binance.Backoff,
	}, log)
	if err != nil {
		return fmt.Errorf("binance connector init: %w", err)
	}
	msgCh, err := wsConn.Stream(ctx)
	if err != nil {
		return fmt.Errorf("ws stream: %w", err)
	}

	// 6) Kafka producer
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
	defer prod.Close()

	// 7) Processor pipeline
	proc := processor.New(
		prod,
		processor.Topics{
			RawTopic:       cfg.Kafka.RawTopic,
			OrderBookTopic: cfg.Kafka.OrderBookTopic,
		},
		log,
	)

	log.Info("collector: all components initialized, starting loops")

	// 8) Run HTTP server and processing loop concurrently
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return httpSrv.Start(ctx)
	})

	g.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case raw, ok := <-msgCh:
				if !ok {
					return fmt.Errorf("ws channel closed")
				}
				if err := proc.Process(ctx, raw); err != nil {
					log.WithContext(ctx).Error("processor error", zap.Error(err))
				}
			}
		}
	})

	// 9) Wait for either loop to exit
	err = g.Wait()
	log.Sync()
	return err
}
