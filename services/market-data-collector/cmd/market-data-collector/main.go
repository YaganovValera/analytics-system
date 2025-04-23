package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/binance"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/kafka"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func main() {
	var cfgFile string

	root := &cobra.Command{
		Use:   "collector",
		Short: "Market Data Collector",
		RunE: func(cmd *cobra.Command, args []string) error {
			viper.SetConfigFile(cfgFile)
			if err := viper.ReadInConfig(); err != nil {
				return err
			}

			// Конфиг читаем в структуры
			symbols := viper.GetStringSlice("binance.symbols")
			wsURL := viper.GetString("binance.ws_url")
			brokers := viper.GetStringSlice("kafka.brokers")
			topic := viper.GetString("kafka.topic")

			// Инициализируем Kafka‑producer
			prod, err := kafka.New(brokers, topic)
			if err != nil {
				return err
			}

			// WS‑канал
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()
			messages, err := binance.Connect(ctx, wsURL, symbols)
			if err != nil {
				return err
			}

			// Основной цикл: читаем WS → шлём в Kafka
			for msg := range messages {
				if err := prod.Publish(ctx, msg); err != nil {
					log.Println("Publish error:", err)
				}
			}
			return nil
		},
	}

	root.Flags().StringVar(&cfgFile, "config", "config/config.yaml", "path to config file")
	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
