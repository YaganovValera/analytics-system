// preprocessor/cmd/preprocessor/main.go
package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/YaganovValera/analytics-system/preprocessor/internal/preprocessor/telemetry"
	kafkac "github.com/YaganovValera/analytics-system/preprocessor/pkg/kafka"
	redisc "github.com/YaganovValera/analytics-system/preprocessor/pkg/redis"
)

var cfgFile string

func initConfig() {
	viper.SetConfigFile(cfgFile)
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		zap.S().Fatalw("Error reading config file", "file", cfgFile, "err", err)
	}
}

func main() {
	root := &cobra.Command{
		Use:   "preprocessor",
		Short: "Market Data Preprocessor service",
		Run: func(cmd *cobra.Command, args []string) {
			// — zap‑логгер
			logger, _ := zap.NewProduction()
			defer logger.Sync()
			sugar := logger.Sugar()

			// — конфиг
			initConfig()
			brokers := viper.GetStringSlice("kafka.brokers")
			groupID := viper.GetString("kafka.group_id")
			topic := viper.GetString("kafka.topic")
			redisAddr := viper.GetString("redis.addr")
			redisPassword := viper.GetString("redis.password")
			redisDB := viper.GetInt("redis.db")
			ttl := viper.GetInt("redis.ttl_seconds")
			otelEndpoint := viper.GetString("telemetry.otel_collector")

			sugar.Infow("Config loaded",
				"kafka_brokers", brokers,
				"kafka_group", groupID,
				"kafka_topic", topic,
				"redis_addr", redisAddr,
				"redis_db", redisDB,
				"redis_ttl", ttl,
				"otel_endpoint", otelEndpoint,
			)

			// — трассировщик
			shutdownTracer, err := telemetry.InitTracer(context.Background(), otelEndpoint, "preprocessor")
			if err != nil {
				sugar.Fatalw("Failed to initialize tracer", "err", err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := shutdownTracer(ctx); err != nil {
					sugar.Errorw("Error shutting down tracer", "err", err)
				}
			}()
			sugar.Infow("OpenTelemetry tracer initialized")

			// — Redis
			cache, err := redisc.New(redisAddr, redisPassword, redisDB, ttl)
			if err != nil {
				sugar.Fatalw("Failed to connect to Redis", "err", err)
			}
			defer cache.Close()
			sugar.Infow("Connected to Redis")

			// — Graceful shutdown context
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			// — Kafka ConsumerGroup
			consumerGroup, err := kafkac.NewConsumerGroup(brokers, groupID, []string{topic})
			if err != nil {
				sugar.Fatalw("Failed to create Kafka consumer group", "err", err)
			}
			defer consumerGroup.Close()

			// — Запуск потребления
			consumerGroup.Start(ctx, func(message []byte) {
				sugar.Infow("Kafka message received", "message", string(message))

				// Парсим JSON, чтобы взять symbol
				var obj map[string]interface{}
				if err := json.Unmarshal(message, &obj); err != nil {
					sugar.Errorw("Failed to unmarshal message", "err", err)
					return
				}
				symbol, ok := obj["s"].(string)
				if !ok {
					sugar.Errorw("Symbol not found in message", "raw", string(message))
					return
				}

				// Пишем в Redis
				if err := cache.StoreRaw(ctx, symbol, message); err != nil {
					sugar.Errorw("Failed to store in Redis", "err", err)
				} else {
					sugar.Infow("Stored to Redis", "key", "raw:"+symbol)
				}
			})

			sugar.Infow("Started Kafka consumer, waiting for messages...")
			<-ctx.Done()
			sugar.Infow("Shutdown signal received, stopping preprocessor")
		},
	}

	root.Flags().StringVar(&cfgFile, "config", "config/config.yaml", "path to config file")
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
