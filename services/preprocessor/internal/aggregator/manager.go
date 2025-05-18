// github.com/YaganovValera/analytics-system/services/preprocessor/internal/aggregator/manager.go

// internal/aggregator/manager.go
package aggregator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

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
	intervals     map[string]time.Duration
	sink          FlushSink
	storage       PartialBarStorage
	log           *logger.Logger
	now           func() time.Time
	flushInterval time.Duration

	ctx    context.Context
	cancel context.CancelFunc
}

func NewManager(intervals []string, sink FlushSink, store PartialBarStorage, log *logger.Logger) (*Manager, error) {
	m := &Manager{
		buckets:       make(map[string]map[string]*candleState),
		intervals:     make(map[string]time.Duration),
		sink:          sink,
		storage:       store,
		log:           log.Named("aggregator"),
		now:           time.Now,
		flushInterval: 1 * time.Second,
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())

	for _, s := range intervals {
		dur, err := IntervalDuration(s)
		if err != nil {
			return nil, fmt.Errorf("invalid interval %q: %w", s, err)
		}
		m.intervals[s] = dur
		m.buckets[s] = make(map[string]*candleState)
	}

	go m.flushLoop()
	return m, nil
}

func (m *Manager) Process(ctx context.Context, data *marketdatapb.MarketData) error {
	ctx, span := tracer.Start(ctx, "Aggregator.Process")
	defer span.End()

	symbol := data.Symbol
	price := data.Price
	volume := data.Volume
	ts := data.Timestamp.AsTime()

	m.mu.Lock()
	defer m.mu.Unlock()

	for interval, dur := range m.intervals {
		bucket := m.buckets[interval]
		state, ok := bucket[symbol]

		if !ok {
			// попытка восстановления из Redis
			existing, err := m.storage.LoadAt(ctx, symbol, interval, ts)

			if err != nil {
				metrics.RestoreErrorsTotal.WithLabelValues(interval).Inc()
				m.log.WithContext(ctx).Warn("restore from redis failed",
					zap.String("symbol", symbol), zap.String("interval", interval), zap.Error(err))
			}
			if existing != nil {
				state = &candleState{Candle: existing, UpdatedAt: ts}
			} else {
				state = newCandleStateWithDuration(symbol, interval, dur, ts, price, volume)
			}
			bucket[symbol] = state
			metrics.ProcessedTotal.WithLabelValues(interval).Inc()
			_ = m.storage.Save(ctx, state.Candle) // best-effort
			continue
		}

		if state.shouldFlush(m.now()) {
			if err := m.flushOne(ctx, interval, symbol, state); err != nil {
				m.log.WithContext(ctx).Error("flush failed",
					zap.String("interval", interval), zap.String("symbol", symbol), zap.Error(err))
			}
			state = newCandleStateWithDuration(symbol, interval, dur, ts, price, volume)
			bucket[symbol] = state
		} else {
			state.update(ts, price, volume)
		}

		metrics.ProcessedTotal.WithLabelValues(interval).Inc()
		_ = m.storage.Save(ctx, state.Candle) // обновляем Redis с TTL
	}

	return nil
}

func (m *Manager) FlushAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for interval, bucket := range m.buckets {
		for symbol, state := range bucket {
			if err := m.flushOne(ctx, interval, symbol, state); err != nil {
				errs = append(errs, err)
				m.log.WithContext(ctx).Error("flushAll failed",
					zap.String("interval", interval),
					zap.String("symbol", symbol),
					zap.Error(err))
			}
			delete(bucket, symbol)
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) Close() error {
	m.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.FlushAll(ctx)
}

func (m *Manager) flushOne(ctx context.Context, interval, symbol string, state *candleState) error {
	ctx, span := tracer.Start(ctx, "Aggregator.flushOne",
		trace.WithAttributes(
			attribute.String("symbol", symbol),
			attribute.String("interval", interval),
		),
	)
	defer span.End()

	state.Candle.Complete = true

	if err := m.sink.FlushCandle(ctx, state.Candle); err != nil {
		return err
	}
	if err := m.storage.DeleteAt(ctx, symbol, interval, state.Candle.Start); err != nil {
		m.log.WithContext(ctx).Warn("delete from redis failed", zap.Error(err))
	}
	metrics.FlushedTotal.WithLabelValues(interval).Inc()
	metrics.FlushLatency.WithLabelValues(interval).Observe(m.now().Sub(state.UpdatedAt).Seconds())
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

	for interval, bucket := range m.buckets {
		for symbol, state := range bucket {
			if state.shouldFlush(now) {
				if err := m.flushOne(m.ctx, interval, symbol, state); err != nil {
					m.log.WithContext(m.ctx).Error("flushExpired failed",
						zap.String("interval", interval),
						zap.String("symbol", symbol),
						zap.Error(err))
				}
				delete(bucket, symbol)
			}
		}
	}
}
