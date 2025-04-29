// services/market-data-collector/internal/processor/processor.go
package processor

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/kafka"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"

	marketdatapb "github.com/YaganovValera/analytics-system/proto/v1/generate/marketdata"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Topics holds Kafka topic names for publishing.
type Topics struct {
	RawTopic       string
	OrderBookTopic string
}

// Processor разбирает RawMessage и публикует в Kafka.
type Processor struct {
	producer *kafka.Producer
	topics   Topics
	log      *logger.Logger
}

// New создает новый Processor.
func New(producer *kafka.Producer, topics Topics, log *logger.Logger) *Processor {
	return &Processor{
		producer: producer,
		topics:   topics,
		log:      log.Named("processor"),
	}
}

// Process обрабатывает одно сообщение raw: парсит JSON, конвертирует в Protobuf и публикует.
// Возвращает ошибку, если публикация не удалась.
func (p *Processor) Process(ctx context.Context, raw binance.RawMessage) error {
	// Увеличиваем общий счётчик событий
	metrics.EventsTotal.Inc()
	start := time.Now()

	switch raw.Type {
	case "trade":
		return p.handleTrade(ctx, raw.Data, start)

	case "depthUpdate":
		return p.handleDepth(ctx, raw.Data, start)

	default:
		p.log.Sugar().Warnw("processor: unsupported event type, skipping", "type", raw.Type)
		return nil
	}
}

func (p *Processor) handleTrade(ctx context.Context, data []byte, start time.Time) error {
	// Структура JSON-ответа Binance для trade
	var evt struct {
		EventType string `json:"e"`
		EventTime int64  `json:"E"`
		Symbol    string `json:"s"`
		TradeTime int64  `json:"T"`
		Price     string `json:"p"`
		Quantity  string `json:"q"`
		TradeID   int64  `json:"t"`
	}
	if err := json.Unmarshal(data, &evt); err != nil {
		p.log.Sugar().Errorw("processor: failed unmarshal trade", "error", err)
		metrics.PublishErrors.Inc()
		return nil // пропускаем эту запись
	}

	// Парсим строки в float64
	price, err := strconv.ParseFloat(evt.Price, 64)
	if err != nil {
		p.log.Sugar().Errorw("processor: invalid price format", "price", evt.Price, "error", err)
		metrics.PublishErrors.Inc()
		return nil
	}
	qty, err := strconv.ParseFloat(evt.Quantity, 64)
	if err != nil {
		p.log.Sugar().Errorw("processor: invalid quantity format", "quantity", evt.Quantity, "error", err)
		metrics.PublishErrors.Inc()
		return nil
	}

	// Формируем Protobuf-сообщение
	msg := &marketdatapb.MarketData{
		Timestamp: timestamppb.New(time.Unix(evt.EventTime/1000, (evt.EventTime%1000)*1e6)),
		Symbol:    evt.Symbol,
		Price:     price,
		BidPrice:  0,
		AskPrice:  0,
		Volume:    qty,
		TradeId:   strconv.FormatInt(evt.TradeID, 10),
	}

	// Сериализация
	payload, err := proto.Marshal(msg)
	if err != nil {
		p.log.Sugar().Errorw("processor: proto marshal error", "error", err)
		metrics.PublishErrors.Inc()
		return nil
	}

	// Публикация
	if err := p.producer.Publish(ctx, p.topics.RawTopic, nil, payload); err != nil {
		p.log.Sugar().Errorw("processor: publish trade error", "error", err)
		metrics.PublishErrors.Inc()
		return err
	}

	// Latency metric
	metrics.PublishLatency.Observe(time.Since(start).Seconds())
	return nil
}

func (p *Processor) handleDepth(ctx context.Context, data []byte, start time.Time) error {
	// Структура JSON-ответа Binance для depthUpdate
	var evt struct {
		EventType string     `json:"e"`
		EventTime int64      `json:"E"`
		Symbol    string     `json:"s"`
		Bids      [][]string `json:"b"` // [price, quantity]
		Asks      [][]string `json:"a"`
	}
	if err := json.Unmarshal(data, &evt); err != nil {
		p.log.Sugar().Errorw("processor: failed unmarshal depth", "error", err)
		metrics.PublishErrors.Inc()
		return nil
	}

	// Конвертация уровней книги
	bids := make([]*marketdatapb.OrderBookLevel, 0, len(evt.Bids))
	for _, lvl := range evt.Bids {
		price, err := strconv.ParseFloat(lvl[0], 64)
		if err != nil {
			continue
		}
		qty, err := strconv.ParseFloat(lvl[1], 64)
		if err != nil {
			continue
		}
		bids = append(bids, &marketdatapb.OrderBookLevel{Price: price, Quantity: qty})
	}
	asks := make([]*marketdatapb.OrderBookLevel, 0, len(evt.Asks))
	for _, lvl := range evt.Asks {
		price, err := strconv.ParseFloat(lvl[0], 64)
		if err != nil {
			continue
		}
		qty, err := strconv.ParseFloat(lvl[1], 64)
		if err != nil {
			continue
		}
		asks = append(asks, &marketdatapb.OrderBookLevel{Price: price, Quantity: qty})
	}

	msg := &marketdatapb.OrderBookSnapshot{
		Timestamp: timestamppb.New(time.Unix(evt.EventTime/1000, (evt.EventTime%1000)*1e6)),
		Symbol:    evt.Symbol,
		Bids:      bids,
		Asks:      asks,
	}

	payload, err := proto.Marshal(msg)
	if err != nil {
		p.log.Sugar().Errorw("processor: proto marshal depth error", "error", err)
		metrics.PublishErrors.Inc()
		return nil
	}

	if err := p.producer.Publish(ctx, p.topics.OrderBookTopic, nil, payload); err != nil {
		p.log.Sugar().Errorw("processor: publish depth error", "error", err)
		metrics.PublishErrors.Inc()
		return err
	}

	metrics.PublishLatency.Observe(time.Since(start).Seconds())
	return nil
}
