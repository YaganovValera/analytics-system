package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/kafka"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"

	marketdatapb "github.com/YaganovValera/analytics-system/proto/v1/marketdata"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var tracer = otel.Tracer("processor")

// Topics хранит имена топиков Kafka.
type Topics struct {
	RawTopic       string
	OrderBookTopic string
}

// processorImpl — приватная реализация Processor.
type processorImpl struct {
	producer kafka.Producer
	topics   Topics
	log      *logger.Logger
}

// New создаёт Processor и возвращает его как интерфейс.
func New(
	producer kafka.Producer,
	topics Topics,
	log *logger.Logger,
) Processor {
	return &processorImpl{
		producer: producer,
		topics:   topics,
		log:      log.Named("processor"),
	}
}

// Process маршрутизирует событие raw по типу.
func (p *processorImpl) Process(ctx context.Context, raw binance.RawMessage) error {
	ctx, span := tracer.Start(ctx, "Process",
		trace.WithAttributes(attribute.String("event.type", raw.Type)))
	defer span.End()

	metrics.EventsTotal.Inc()
	start := time.Now()

	switch raw.Type {
	case "trade":
		return p.handleTrade(ctx, raw.Data, start)
	case "depthUpdate":
		return p.handleDepth(ctx, raw.Data, start)
	default:
		metrics.UnsupportedEvents.Inc()
		p.log.WithContext(ctx).Debug("unsupported event, skipping",
			zap.String("type", raw.Type))
		return nil
	}
}

func (p *processorImpl) handleTrade(ctx context.Context, data []byte, start time.Time) error {
	ctx, span := tracer.Start(ctx, "HandleTrade")
	defer span.End()

	evt, err := parseTrade(data)
	if err != nil {
		metrics.ParseErrors.Inc()
		p.log.WithContext(ctx).Error("parse trade failed", zap.Error(err))
		span.RecordError(err)
		return nil
	}

	msg := &marketdatapb.MarketData{
		Timestamp: timestamppb.New(time.UnixMilli(evt.EventTime)),
		Symbol:    evt.Symbol,
		Price:     evt.Price,
		Volume:    evt.Quantity,
		TradeId:   evt.TradeID,
	}

	payload, err := proto.Marshal(msg)
	if err != nil {
		metrics.SerializeErrors.Inc()
		p.log.WithContext(ctx).Error("marshal trade proto failed", zap.Error(err))
		span.RecordError(err)
		return nil
	}

	if err := p.producer.Publish(ctx, p.topics.RawTopic, nil, payload); err != nil {
		metrics.PublishErrors.Inc()
		p.log.WithContext(ctx).Error("publish trade failed", zap.Error(err))
		span.RecordError(err)
		return err
	}

	metrics.PublishLatency.Observe(time.Since(start).Seconds())
	return nil
}

type tradeEvent struct {
	EventTime int64
	Symbol    string
	Price     float64
	Quantity  float64
	TradeID   string
}

func parseTrade(data []byte) (*tradeEvent, error) {
	var raw struct {
		E  string `json:"e"`
		Ev int64  `json:"E"`
		S  string `json:"s"`
		P  string `json:"p"`
		Q  string `json:"q"`
		T  int64  `json:"t"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	price, err := strconv.ParseFloat(raw.P, 64)
	if err != nil {
		return nil, fmt.Errorf("price=%q: %w", raw.P, err)
	}
	qty, err := strconv.ParseFloat(raw.Q, 64)
	if err != nil {
		return nil, fmt.Errorf("quantity=%q: %w", raw.Q, err)
	}
	return &tradeEvent{
		EventTime: raw.Ev,
		Symbol:    raw.S,
		Price:     price,
		Quantity:  qty,
		TradeID:   strconv.FormatInt(raw.T, 10),
	}, nil
}

func (p *processorImpl) handleDepth(ctx context.Context, data []byte, start time.Time) error {
	ctx, span := tracer.Start(ctx, "HandleDepth")
	defer span.End()

	evt, err := parseDepth(data)
	if err != nil {
		metrics.ParseErrors.Inc()
		p.log.WithContext(ctx).Error("parse depth failed", zap.Error(err))
		span.RecordError(err)
		return nil
	}

	msg := &marketdatapb.OrderBookSnapshot{
		Timestamp: timestamppb.New(time.UnixMilli(evt.EventTime)),
		Symbol:    evt.Symbol,
		Bids:      evt.Bids,
		Asks:      evt.Asks,
	}

	payload, err := proto.Marshal(msg)
	if err != nil {
		metrics.SerializeErrors.Inc()
		p.log.WithContext(ctx).Error("marshal depth proto failed", zap.Error(err))
		span.RecordError(err)
		return nil
	}

	if err := p.producer.Publish(ctx, p.topics.OrderBookTopic, nil, payload); err != nil {
		metrics.PublishErrors.Inc()
		p.log.WithContext(ctx).Error("publish depth failed", zap.Error(err))
		span.RecordError(err)
		return err
	}

	metrics.PublishLatency.Observe(time.Since(start).Seconds())
	return nil
}

type depthEvent struct {
	EventTime int64
	Symbol    string
	Bids      []*marketdatapb.OrderBookLevel
	Asks      []*marketdatapb.OrderBookLevel
}

func parseDepth(data []byte) (*depthEvent, error) {
	var raw struct {
		E  string     `json:"e"`
		Ev int64      `json:"E"`
		S  string     `json:"s"`
		B  [][]string `json:"b"`
		A  [][]string `json:"a"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	convert := func(pair []string) (*marketdatapb.OrderBookLevel, error) {
		price, err := strconv.ParseFloat(pair[0], 64)
		if err != nil {
			return nil, fmt.Errorf("price=%q: %w", pair[0], err)
		}
		qty, err := strconv.ParseFloat(pair[1], 64)
		if err != nil {
			return nil, fmt.Errorf("quantity=%q: %w", pair[1], err)
		}
		return &marketdatapb.OrderBookLevel{Price: price, Quantity: qty}, nil
	}

	bids := make([]*marketdatapb.OrderBookLevel, 0, len(raw.B))
	for _, p := range raw.B {
		if lvl, err := convert(p); err == nil {
			bids = append(bids, lvl)
		}
	}
	asks := make([]*marketdatapb.OrderBookLevel, 0, len(raw.A))
	for _, p := range raw.A {
		if lvl, err := convert(p); err == nil {
			asks = append(asks, lvl)
		}
	}
	return &depthEvent{
		EventTime: raw.Ev,
		Symbol:    raw.S,
		Bids:      bids,
		Asks:      asks,
	}, nil
}
