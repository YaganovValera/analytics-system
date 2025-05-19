// github.com/YaganovValera/analytics-system/services/preprocessor/internal/aggregator/manager.go
// services/preprocessor/internal/aggregator/manager.go
package aggregator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/YaganovValera/analytics-system/common/interval"
	"github.com/YaganovValera/analytics-system/common/logger"
	marketdatapb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/marketdata"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/metrics"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var tracer = otel.Tracer("preprocessor/aggregator")

// Manager реализует Aggregator.
type Manager struct {
	mu            sync.RWMutex
	buckets       map[string]map[string]*candleState // interval -> symbol -> state
	intervals     []string                           // list of interval strings
	sink          FlushSink
	storage       PartialBarStorage
	log           *logger.Logger
	now           func() time.Time
	flushInterval time.Duration

	ctx    context.Context
	cancel context.CancelFunc
}

// NewManager создаёт и валидацирует Manager.
func NewManager(intervals []string, sink FlushSink, store PartialBarStorage, log *logger.Logger) (*Manager, error) {
	m := &Manager{
		buckets:       make(map[string]map[string]*candleState),
		intervals:     intervals,
		sink:          sink,
		storage:       store,
		log:           log.Named("aggregator"),
		now:           time.Now,
		flushInterval: time.Second,
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())

	// Проверяем корретность интервалов и инициализируем бакеты
	for _, iv := range intervals {
		if _, err := interval.Duration(interval.Interval(iv)); err != nil {
			return nil, fmt.Errorf("invalid interval %q: %w", iv, err)
		}
		m.buckets[iv] = make(map[string]*candleState)
	}

	go m.flushLoop()
	return m, nil
}

// Process обрабатывает один рыночный тик.
func (m *Manager) Process(ctx context.Context, data *marketdatapb.MarketData) error {
	ctx, span := tracer.Start(ctx, "Aggregator.Process")
	defer span.End()

	symbol := data.Symbol
	price := data.Price
	volume := data.Volume
	ts := data.Timestamp.AsTime()

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, iv := range m.intervals {
		bucket := m.buckets[iv]
		state, ok := bucket[symbol]

		if !ok {
			// восстановление или создание нового
			existing, err := m.storage.LoadAt(ctx, symbol, iv, ts)
			if err != nil {
				metrics.RestoreErrorsTotal.WithLabelValues(iv).Inc()
				m.log.WithContext(ctx).Warn("restore from redis failed",
					zap.String("symbol", symbol), zap.String("interval", iv), zap.Error(err))
			}
			if existing != nil {
				state = &candleState{Candle: existing, UpdatedAt: ts}
			} else {
				state = newCandleState(symbol, iv, ts, price, volume)
			}
			bucket[symbol] = state
			metrics.ProcessedTotal.WithLabelValues(iv).Inc()
			_ = m.storage.Save(ctx, state.Candle)
			continue
		}

		// проверка необходимости сброса
		if state.shouldFlush(m.now()) {
			if err := m.flushOne(ctx, iv, symbol, state); err != nil {
				m.log.WithContext(ctx).Error("flush failed",
					zap.String("interval", iv), zap.String("symbol", symbol), zap.Error(err))
			}
			state = newCandleState(symbol, iv, ts, price, volume)
			bucket[symbol] = state
		} else {
			state.update(ts, price, volume)
		}

		metrics.ProcessedTotal.WithLabelValues(iv).Inc()
		_ = m.storage.Save(ctx, state.Candle)
	}

	return nil
}

// FlushAll сбрасывает все in-progress свечи.
func (m *Manager) FlushAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for iv, bucket := range m.buckets {
		for symbol, state := range bucket {
			if err := m.flushOne(ctx, iv, symbol, state); err != nil {
				errs = append(errs, err)
				m.log.WithContext(ctx).Error("flushAll failed",
					zap.String("interval", iv), zap.String("symbol", symbol), zap.Error(err))
			}
			delete(bucket, symbol)
		}
	}
	return errors.Join(errs...)
}

// Close завершает работу агрегатора.
func (m *Manager) Close() error {
	m.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.FlushAll(ctx)
}

func (m *Manager) flushOne(ctx context.Context, iv, symbol string, state *candleState) error {
	ctx, span := tracer.Start(ctx, "Aggregator.flushOne",
		trace.WithAttributes(
			attribute.String("symbol", symbol),
			attribute.String("interval", iv),
		),
	)
	defer span.End()

	state.Candle.Complete = true
	if err := m.sink.FlushCandle(ctx, state.Candle); err != nil {
		return err
	}
	if err := m.storage.DeleteAt(ctx, symbol, iv, state.Candle.Start); err != nil {
		m.log.WithContext(ctx).Warn("delete from redis failed", zap.Error(err))
	}
	metrics.FlushedTotal.WithLabelValues(iv).Inc()
	latency := m.now().Sub(state.UpdatedAt).Seconds()
	metrics.FlushLatency.WithLabelValues(iv).Observe(latency)
	return nil
}

func (m *Manager) flushLoop() {
	ticker := time.NewTicker(m.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.flushExpired()
		case <-m.ctx.Done():
			m.log.Info("flushLoop: exiting on context cancel")
			return
		}
	}
}

func (m *Manager) flushExpired() {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()

	for iv, bucket := range m.buckets {
		for symbol, state := range bucket {
			if state.shouldFlush(now) {
				if err := m.flushOne(m.ctx, iv, symbol, state); err != nil {
					m.log.WithContext(m.ctx).Error("flushExpired failed",
						zap.String("interval", iv), zap.String("symbol", symbol), zap.Error(err))
				}
				delete(bucket, symbol)
			}
		}
	}
}
