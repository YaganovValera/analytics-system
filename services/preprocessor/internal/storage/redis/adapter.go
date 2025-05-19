// services/preprocessor/internal/storage/redis/adapter.go
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/interval"
	"github.com/YaganovValera/analytics-system/common/redis"
	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/aggregator"

	"go.opentelemetry.io/otel"
	"google.golang.org/protobuf/proto"
)

var tracer = otel.Tracer("preprocessor/storage/redis")

type Adapter struct {
	client redis.Client
}

func NewAdapter(client redis.Client) *Adapter {
	return &Adapter{client: client}
}

func (a *Adapter) Save(ctx context.Context, c *aggregator.Candle) error {
	ctx, span := tracer.Start(ctx, "Redis.Save")
	defer span.End()
	key := fmt.Sprintf("ohlcv:%s:%s:%s",
		c.Symbol, c.Interval, c.Start.UTC().Format(time.RFC3339))
	data, err := proto.Marshal(c.ToProto())
	if err != nil {
		return err
	}
	// TTL = 2 × duration
	dur, err := interval.Duration(interval.Interval(c.Interval))
	if err != nil {
		return fmt.Errorf("invalid interval %q: %w", c.Interval, err)
	}
	return a.client.Set(ctx, key, string(data), 2*dur)
}

func (a *Adapter) LoadAt(ctx context.Context, symbol, iv string, ts time.Time) (*aggregator.Candle, error) {
	ctx, span := tracer.Start(ctx, "Redis.LoadAt")
	defer span.End()
	start, err := aggregator.AlignToInterval(ts, iv)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("ohlcv:%s:%s:%s", symbol, iv, start.UTC().Format(time.RFC3339))
	raw, err := a.client.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var pb analyticspb.Candle
	if err := proto.Unmarshal([]byte(raw), &pb); err != nil {
		return nil, err
	}
	return &aggregator.Candle{
		Symbol:   pb.Symbol,
		Interval: iv,
		Start:    pb.OpenTime.AsTime(),
		End:      pb.CloseTime.AsTime(),
		Open:     pb.Open,
		High:     pb.High,
		Low:      pb.Low,
		Close:    pb.Close,
		Volume:   pb.Volume,
	}, nil
}

func (a *Adapter) DeleteAt(ctx context.Context, symbol, iv string, ts time.Time) error {
	ctx, span := tracer.Start(ctx, "Redis.DeleteAt")
	defer span.End()
	start, err := aggregator.AlignToInterval(ts, iv)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("ohlcv:%s:%s:%s", symbol, iv, start.UTC().Format(time.RFC3339))
	_, err = a.client.Del(ctx, key)
	return err
}

func (a *Adapter) Close() error {
	return a.client.Close()
}

func (a *Adapter) Load(ctx context.Context, symbol, iv string) (*aggregator.Candle, error) {
	return a.LoadAt(ctx, symbol, iv, time.Now().UTC())
}

func (a *Adapter) Delete(ctx context.Context, symbol, iv string) error {
	return a.DeleteAt(ctx, symbol, iv, time.Now().UTC())
}
