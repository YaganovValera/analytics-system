// github.com/YaganovValera/analytics-system/services/preprocessor/internal/transport/marshaler.go

package transport

import (
	"context"
	"fmt"

	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/aggregator"
	"google.golang.org/protobuf/proto"
)

// MarshalCandleToKafka сериализует агрегированную свечу в []byte для Kafka.
func MarshalCandleToKafka(ctx context.Context, c *aggregator.Candle) ([]byte, error) {
	pb := c.ToProto()
	return proto.Marshal(pb)
}

// MarshalCandleToInsertArgs возвращает значения для TimescaleDB.
func MarshalCandleToInsertArgs(c *aggregator.Candle) (query string, args []interface{}) {
	query = `INSERT INTO candles (
		time, symbol, interval, open, high, low, close, volume
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	ON CONFLICT (symbol, interval, time) DO NOTHING`

	args = []interface{}{
		c.Start.UTC(),
		c.Symbol,
		c.Interval,
		c.Open,
		c.High,
		c.Low,
		c.Close,
		c.Volume,
	}
	return
}

// UnmarshalCandleFromBytes парсит []byte в protobuf Candle.
func UnmarshalCandleFromBytes(data []byte) (*analyticspb.Candle, error) {
	var pb analyticspb.Candle
	if err := proto.Unmarshal(data, &pb); err != nil {
		return nil, fmt.Errorf("unmarshal candle: %w", err)
	}
	return &pb, nil
}
