// github.com/YaganovValera/analytics-system/services/preprocessor/internal/storage/redis/redis.go

package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/YaganovValera/analytics-system/common/logger"
	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	"github.com/YaganovValera/analytics-system/services/preprocessor/internal/aggregator"
	"github.com/go-redis/redis/v8"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

var tracer = otel.Tracer("preprocessor/storage/redis")

// RedisStorage реализует PartialBarStorage через go-redis.
type RedisStorage struct {
	rdb *redis.Client
	log *logger.Logger
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

func NewRedisStorage(cfg RedisConfig, log *logger.Logger) (*RedisStorage, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis: ping failed: %w", err)
	}

	return &RedisStorage{rdb: rdb, log: log.Named("redis")}, nil
}

func redisKey(symbol, interval string, start time.Time) string {
	return fmt.Sprintf("ohlcv:%s:%s:%s", symbol, interval, start.UTC().Format("2006-01-02T15:04:05Z"))
}

func (s *RedisStorage) Save(ctx context.Context, c *aggregator.Candle) error {
	ctx, span := tracer.Start(ctx, "Redis.Save")
	defer span.End()

	data, err := proto.Marshal(c.ToProto())
	if err != nil {
		s.log.WithContext(ctx).Error("marshal failed", zap.Error(err))
		return fmt.Errorf("redis save: marshal: %w", err)
	}

	dur, err := aggregator.IntervalDuration(c.Interval)
	if err != nil {
		return fmt.Errorf("invalid interval duration: %w", err)
	}

	ttl := 2 * dur
	key := redisKey(c.Symbol, c.Interval, c.Start)

	if err := s.rdb.Set(ctx, key, data, ttl).Err(); err != nil {
		s.log.WithContext(ctx).Error("redis set failed", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("redis set failed: %w", err)
	}
	return nil
}

func (s *RedisStorage) LoadAt(ctx context.Context, symbol, interval string, ts time.Time) (*aggregator.Candle, error) {
	ctx, span := tracer.Start(ctx, "Redis.LoadAt")
	defer span.End()

	dur, err := aggregator.IntervalDuration(interval)
	if err != nil {
		return nil, fmt.Errorf("invalid interval duration: %w", err)
	}

	start := aggregator.AlignToInterval(ts, dur)
	key := redisKey(symbol, interval, start)
	val, err := s.rdb.Get(ctx, key).Bytes()

	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		s.log.WithContext(ctx).Error("redis get failed", zap.String("key", key), zap.Error(err))
		return nil, fmt.Errorf("redis get failed: %w", err)
	}

	var pb analyticspb.Candle
	if err := proto.Unmarshal(val, &pb); err != nil {
		s.log.WithContext(ctx).Error("unmarshal failed", zap.Error(err))
		return nil, fmt.Errorf("unmarshal failed: %w", err)
	}

	return &aggregator.Candle{
		Symbol:   pb.Symbol,
		Interval: interval,
		Start:    pb.OpenTime.AsTime(),
		End:      pb.CloseTime.AsTime(),
		Open:     pb.Open,
		High:     pb.High,
		Low:      pb.Low,
		Close:    pb.Close,
		Volume:   pb.Volume,
		Complete: false,
	}, nil
}

func (s *RedisStorage) DeleteAt(ctx context.Context, symbol, interval string, ts time.Time) error {
	ctx, span := tracer.Start(ctx, "Redis.DeleteAt")
	defer span.End()

	dur, err := aggregator.IntervalDuration(interval)
	if err != nil {
		return fmt.Errorf("invalid interval duration: %w", err)
	}

	start := aggregator.AlignToInterval(ts, dur)
	key := redisKey(symbol, interval, start)
	if err := s.rdb.Del(ctx, key).Err(); err != nil {
		s.log.WithContext(ctx).Error("redis delete failed", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("redis delete failed: %w", err)
	}
	return nil
}

// Optional fallback — оставлены как обёртки на случай совместимости
func (s *RedisStorage) Load(ctx context.Context, symbol, interval string) (*aggregator.Candle, error) {
	return s.LoadAt(ctx, symbol, interval, time.Now().UTC())
}

func (s *RedisStorage) Delete(ctx context.Context, symbol, interval string) error {
	return s.DeleteAt(ctx, symbol, interval, time.Now().UTC())
}

func (s *RedisStorage) Close() error {
	return s.rdb.Close()
}
