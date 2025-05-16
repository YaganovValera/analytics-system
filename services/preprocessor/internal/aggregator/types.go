// github.com/YaganovValera/analytics-system/services/preprocessor/internal/aggregator/type.go

package aggregator

import (
	"fmt"
	"strings"
	"time"

	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// Candle представляет агрегированную OHLCV свечу (внутренний формат).
type Candle struct {
	Symbol   string
	Interval string    // e.g. "1m", "5m"
	Start    time.Time // начало бара (всегда округлено вниз)
	End      time.Time // конец интервала
	Open     float64
	High     float64
	Low      float64
	Close    float64
	Volume   float64
	Complete bool // завершён ли бар
}

// ToProto конвертирует Candle в protobuf.
func (c *Candle) ToProto() *analyticspb.Candle {
	return &analyticspb.Candle{
		Symbol:    c.Symbol,
		Open:      c.Open,
		High:      c.High,
		Low:       c.Low,
		Close:     c.Close,
		Volume:    c.Volume,
		OpenTime:  timestamppb.New(c.Start),
		CloseTime: timestamppb.New(c.End),
	}
}

// IntervalDuration возвращает длительность интервала.
func IntervalDuration(interval string) (time.Duration, error) {
	switch strings.ToLower(interval) {
	case "1m":
		return time.Minute, nil
	case "5m":
		return 5 * time.Minute, nil
	case "15m":
		return 15 * time.Minute, nil
	case "1h":
		return time.Hour, nil
	case "4h":
		return 4 * time.Hour, nil
	case "1d":
		return 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid interval: %s", interval)
	}
}

// AlignToInterval округляет время вниз до начала интервала.
func AlignToInterval(ts time.Time, interval time.Duration) time.Time {
	d := ts.Truncate(interval)
	return d
}

// IntervalList валидирует список интервалов из config.
func IntervalList(raw []string) ([]time.Duration, error) {
	var result []time.Duration
	for _, s := range raw {
		dur, err := IntervalDuration(s)
		if err != nil {
			return nil, err
		}
		result = append(result, dur)
	}
	return result, nil
}
