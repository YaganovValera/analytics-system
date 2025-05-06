// services/market-data-collector/pkg/kafka/producer_test.go
package kafka

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/backoff"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"
)

// Проверяем applyDefaults и validate.
func TestConfigDefaultsAndValidate(t *testing.T) {
	cases := []struct {
		name     string
		input    Config
		wantErr  bool
		wantAcks string
		wantComp string
	}{
		{"empty", Config{}, true, "all", "none"},
		{"noBrokers", Config{Compression: "gzip"}, true, "all", "gzip"},
		{"ok", Config{Brokers: []string{"b1"}}, false, "all", "none"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := c.input
			cfg.applyDefaults()
			if got := cfg.RequiredAcks; got != c.wantAcks {
				t.Errorf("RequiredAcks = %q; want %q", got, c.wantAcks)
			}
			if got := cfg.Compression; got != c.wantComp {
				t.Errorf("Compression = %q; want %q", got, c.wantComp)
			}
			err := cfg.validate()
			if (err != nil) != c.wantErr {
				t.Errorf("validate() error = %v; wantErr=%v", err, c.wantErr)
			}
		})
	}
}

// Проверяем buildSaramaConfig для acks.
func TestBuildSaramaConfig_RequiredAcks(t *testing.T) {
	cases := []struct {
		acks    string
		wantErr bool
	}{
		{"all", false}, {"leader", false}, {"none", false},
		{"ALL", false}, {"LeAdEr", false}, {"invalid", true},
	}
	for _, c := range cases {
		t.Run(c.acks, func(t *testing.T) {
			cfg := Config{RequiredAcks: c.acks, Compression: "none", Brokers: []string{"x"}}
			sc, err := buildSaramaConfig(cfg)
			if c.wantErr {
				if err == nil {
					t.Errorf("buildSaramaConfig(%q) expected error", c.acks)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// проверяем, что RequiredAcks правильно выставился
			switch strings.ToLower(c.acks) {
			case "all":
				if sc.Producer.RequiredAcks != sarama.WaitForAll {
					t.Errorf("got %v; want %v", sc.Producer.RequiredAcks, sarama.WaitForAll)
				}
			case "leader":
				if sc.Producer.RequiredAcks != sarama.WaitForLocal {
					t.Errorf("got %v; want %v", sc.Producer.RequiredAcks, sarama.WaitForLocal)
				}
			case "none":
				if sc.Producer.RequiredAcks != sarama.NoResponse {
					t.Errorf("got %v; want %v", sc.Producer.RequiredAcks, sarama.NoResponse)
				}
			}
		})
	}
}

// Проверяем buildSaramaConfig для Compression.
func TestBuildSaramaConfig_Compression(t *testing.T) {
	cases := []struct {
		comp    string
		wantErr bool
	}{
		{"none", false}, {"gzip", false}, {"snappy", false},
		{"lz4", false}, {"zstd", false}, {"NONE", false},
		{"bogus", true},
	}
	for _, c := range cases {
		t.Run(c.comp, func(t *testing.T) {
			cfg := Config{RequiredAcks: "all", Compression: c.comp, Brokers: []string{"x"}}
			_, err := buildSaramaConfig(cfg)
			if c.wantErr {
				if err == nil {
					t.Errorf("buildSaramaConfig comp=%q expected error", c.comp)
				}
			} else if err != nil {
				t.Fatalf("unexpected error for %q: %v", c.comp, err)
			}
		})
	}
}

// Проверяем Publish: сначала возвращаем ошибку, потом — успех.
func TestPublish_RetryAndSuccess(t *testing.T) {
	// мок SyncProducer
	saramaConfig := sarama.NewConfig()
	mockProd := mocks.NewSyncProducer(t, saramaConfig)
	defer mockProd.Close()

	mockProd.ExpectSendMessageAndFail(sarama.ErrOutOfBrokers)
	mockProd.ExpectSendMessageAndSucceed()

	log, _ := logger.New(logger.Config{Level: "debug", DevMode: true})
	kp := &kafkaProducer{
		prod:       mockProd,
		logger:     log,
		backoffCfg: backoff.Config{InitialInterval: 1 * time.Millisecond, Multiplier: 1, MaxInterval: 1 * time.Millisecond, MaxElapsedTime: 50 * time.Millisecond},
	}
	if err := kp.Publish(context.Background(), "topic", []byte("key"), []byte("value")); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
}

// Проверяем, что NewProducer отрабатывает ошибку валидации до Sarama.
func TestNewProducer_InvalidConfig(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "debug", DevMode: true})
	if _, err := NewProducer(context.Background(), Config{}, log); err == nil {
		t.Fatal("Expected error for empty Config, got nil")
	}
}

// Проверяем, что NewProducer отказывает на неверном RequiredAcks.
func TestNewProducer_InvalidAcks(t *testing.T) {
	cfg := Config{
		Brokers:      []string{"dummy"},
		RequiredAcks: "invalid",
		Compression:  "none",
	}
	log, _ := logger.New(logger.Config{Level: "debug", DevMode: true})
	if _, err := NewProducer(context.Background(), cfg, log); err == nil {
		t.Fatal("Expected error for invalid RequiredAcks, got nil")
	}
}
