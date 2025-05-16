// github.com/YaganovValera/analytics-system/services/preprocessor/internal/aggregator/candle.go

package aggregator

import (
	"time"
)

// candleState — состояние in-progress бара по symbol+interval.
type candleState struct {
	Candle    *Candle
	UpdatedAt time.Time // последнее обновление
}

// update обновляет текущий бар новой точкой.
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

// shouldFlush определяет, пора ли завершить бар.
func (cs *candleState) shouldFlush(now time.Time) bool {
	return !now.Before(cs.Candle.End)
}

func newCandleStateWithDuration(symbol, interval string, dur time.Duration, ts time.Time, price, volume float64) *candleState {
	start := AlignToInterval(ts, dur)
	end := start.Add(dur)
	return &candleState{
		Candle: &Candle{
			Symbol:   symbol,
			Interval: interval,
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
