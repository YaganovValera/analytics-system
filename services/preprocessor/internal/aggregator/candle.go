// preprocessor/internal/aggregator/candle.go
package aggregator

import (
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/interval"
)

// candleState — состояние in-progress бара.
type candleState struct {
	Candle    *Candle
	UpdatedAt time.Time
}

func (cs *candleState) update(ts time.Time, price, volume float64) {
	c := cs.Candle
	if price > c.High {
		c.High = price
	}
	if price < c.Low {
		c.Low = price
	}
	c.Close = price
	c.Volume += volume
	cs.UpdatedAt = ts
}

func (cs *candleState) shouldFlush(now time.Time) bool {
	return !now.Before(cs.Candle.End)
}

// newCandleState создаёт состояние нового бара.
func newCandleState(symbol, iv string, ts time.Time, price, volume float64) *candleState {
	start, err := AlignToInterval(ts, iv)
	if err != nil {
		panic(fmt.Sprintf("invalid interval %q: %v", iv, err))
	}
	dur, err := interval.Duration(interval.Interval(iv))
	if err != nil {
		panic(fmt.Sprintf("invalid interval %q: %v", iv, err))
	}
	end := start.Add(dur)
	return &candleState{
		Candle: &Candle{
			Symbol:   symbol,
			Interval: iv,
			Start:    start,
			End:      end,
			Open:     price,
			High:     price,
			Low:      price,
			Close:    price,
			Volume:   volume,
			Complete: false,
		},
		UpdatedAt: ts,
	}
}
