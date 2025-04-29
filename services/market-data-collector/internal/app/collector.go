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
	pkgtelemetry "github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/telemetry"

	"golang.org/x/sync/errgroup"
)

// Run запускает market-data-collector: инициализирует метрики, телеметрию, HTTP-сервер,
// WebSocket-коннектор и конвейер обработки сообщений в Kafka.
func Run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	// 1. Распечатать конфигурацию
	if err := cfg.Print(); err != nil {
		log.Sugar().Warnw("config: failed to print configuration", "error", err)
	}

	// 2. Регистрация метрик
	metrics.Register()

	// 3. Инициализация TracerProvider
	tracerShutdown, err := pkgtelemetry.InitTracer(
		ctx,
		cfg.Telemetry.OTLPEndpoint,
		cfg.ServiceName,
		cfg.ServiceVersion,
		cfg.Telemetry.Insecure,
		log,
	)
	if err != nil {
		return fmt.Errorf("telemetry init: %w", err)
	}
	defer tracerShutdown(ctx)

	// 4. Создание HTTP-сервера
	readiness := func() error { return nil }
	httpSrv := httpserver.NewServer(
		fmt.Sprintf(":%d", cfg.HTTP.HealthPort),
		readiness,
		log,
	)

	// 5. WebSocket-коннектор
	wsConn, err := binance.NewConnector(binance.Config{
		WSURL:         cfg.Binance.WSURL,
		Symbols:       cfg.Binance.Symbols,
		BufferSize:    cfg.Buffer.Size,
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

	// 6. Kafka-продьюсер
	producer, err := kafka.NewProducer(ctx, kafka.Config{
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
	defer producer.Close()

	// 7. Processor
	proc := processor.New(
		producer,
		processor.Topics{RawTopic: cfg.Kafka.Topics.Raw, OrderBookTopic: cfg.Kafka.Topics.OrderBook},
		log,
	)

	// 8. Errgroup для HTTP и pipeline
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
					log.Sugar().Errorw("processor error", "error", err)
				}
			}
		}
	})

	// 9. Ожидание завершения
	err = g.Wait()
	log.Sync()
	return err
}
