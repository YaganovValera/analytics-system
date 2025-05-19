// github.com/YaganovValera/analytics-system/services/preprocessor/internal/aggregator/type.go

package aggregator

import (
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/interval"
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

// AlignToInterval округляет ts вниз до ближайшей границы interval.
// Использует common/interval.
func AlignToInterval(ts time.Time, iv string) (time.Time, error) {
	i := interval.Interval(iv)
	dur, err := interval.Duration(i)
	if err != nil {
		return ts, fmt.Errorf("invalid interval %q: %w", iv, err)
	}
	return ts.Truncate(dur), nil
}
