// services/market-data-collector/pkg/binance/ws_test.go
package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/backoff"
	"github.com/YaganovValera/analytics-system/services/market-data-collector/pkg/logger"
	"github.com/gorilla/websocket"
)

// Проверяем applyDefaults и validate на разных комбинациях.
func TestConfigDefaultsAndValidate(t *testing.T) {
	cases := []struct {
		name     string
		input    Config
		wantErr  bool
		wantBuf  int
		wantRead time.Duration
		wantSub  time.Duration
	}{
		{"empty", Config{}, true, 100, 30 * time.Second, 5 * time.Second},
		{"noStreams", Config{URL: "ws://foo"}, true, 100, 30 * time.Second, 5 * time.Second},
		{"ok", Config{URL: "ws://foo", Streams: []string{"s"}}, false, 100, 30 * time.Second, 5 * time.Second},
		{"custom", Config{
			URL: "u", Streams: []string{"s"},
			BufferSize: 5, ReadTimeout: 7 * time.Second, SubscribeTimeout: 3 * time.Second,
		}, false, 5, 7 * time.Second, 3 * time.Second},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := c.input
			cfg.applyDefaults()
			if got := cfg.BufferSize; got != c.wantBuf {
				t.Errorf("BufferSize = %v; want %v", got, c.wantBuf)
			}
			if got := cfg.ReadTimeout; got != c.wantRead {
				t.Errorf("ReadTimeout = %v; want %v", got, c.wantRead)
			}
			if got := cfg.SubscribeTimeout; got != c.wantSub {
				t.Errorf("SubscribeTimeout = %v; want %v", got, c.wantSub)
			}
			err := cfg.validate()
			if (err != nil) != c.wantErr {
				t.Errorf("validate() error = %v; wantErr %v", err, c.wantErr)
			}
		})
	}
}

// Интеграционный тест Stream() c реальным WebSocket-сервером.
func TestConnector_StreamIntegration(t *testing.T) {
	// 1) Заводим тестовый WS-сервер, который примет SUBSCRIBE и отдаст одно сообщение, потом закроется.
	upg := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upg.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		defer conn.Close()

		// ждём запрос SUBSCRIBE
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read subscribe: %v", err)
		}
		if !strings.Contains(string(msg), `"method":"SUBSCRIBE"`) {
			t.Fatalf("expected subscribe, got %s", msg)
		}

		// шлём тестовое событие
		env := map[string]interface{}{
			"data": map[string]interface{}{"e": "testEvent", "foo": 123},
		}
		if err := conn.WriteJSON(env); err != nil {
			t.Fatalf("write json: %v", err)
		}
		// и сразу закрываем
	}))
	defer server.Close()

	// 2) Коннектор
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	cfg := Config{URL: wsURL, Streams: []string{"s"}}
	// делаем бэкофф очень быстрым
	cfg.BackoffConfig = backoff.Config{InitialInterval: 1 * time.Millisecond, RandomizationFactor: 0, Multiplier: 1, MaxInterval: 1 * time.Millisecond, MaxElapsedTime: 10 * time.Millisecond}
	log, _ := logger.New(logger.Config{Level: "debug", DevMode: true})
	conn, err := NewConnector(cfg, log)
	if err != nil {
		t.Fatalf("NewConnector: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ch, err := conn.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	// 3) Читаем из канала до закрытия
	var got RawMessage
	for m := range ch {
		got = m
	}

	if got.Type != "testEvent" {
		t.Errorf("RawMessage.Type = %q; want %q", got.Type, "testEvent")
	}
	if !strings.Contains(string(got.Data), `"foo":123`) {
		t.Errorf("RawMessage.Data = %s; want contains foo", got.Data)
	}
}
