// services/preprocessor/internal/processor/processor.go
package processor

import (
	"context"
	"time"

	"github.com/YaganovValera/analytics-system/common/kafka"
	"github.com/YaganovValera/analytics-system/common/logger"
	marketdatapb "github.com/YaganovValera/analytics-system/proto/v1/marketdata"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/storage"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// Processor записывает все сырые события в TimescaleDB.
type Processor struct {
	storage *storage.PostgresStorage
	log     *logger.Logger
}

// NewProcessor принимает на вход PostgresStorage и логгер.
func NewProcessor(storage *storage.PostgresStorage, log *logger.Logger) *Processor {
	return &Processor{
		storage: storage,
		log:     log.Named("processor"),
	}
}

// Process десериализует сообщение и вставляет его в raw_market_data.
func (p *Processor) Process(ctx context.Context, msg *kafka.Message) error {
	metrics.ProcessedMessages.Inc()

	var m marketdatapb.MarketData
	if err := proto.Unmarshal(msg.Value, &m); err != nil {
		metrics.ProcessErrors.Inc()
		p.log.WithContext(ctx).Error("unmarshal MarketData", zap.Error(err))
		return nil // считаем некритичной, продолжаем поток
	}

	start := time.Now()
	if err := p.storage.InsertRaw(ctx, &m); err != nil {
		metrics.InsertErrors.Inc()
		p.log.WithContext(ctx).Error("insert raw_market_data failed", zap.Error(err))
		return err
	}
	metrics.InsertLatency.Observe(time.Since(start).Seconds())
	return nil
}
