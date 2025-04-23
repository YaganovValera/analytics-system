package binance

import (
	"context"
	"log"

	"github.com/gorilla/websocket"
)

func Connect(ctx context.Context, url string, symbols []string) (<-chan []byte, error) {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}
	ch := make(chan []byte)
	go func() {
		defer conn.Close()
		// подписка
		msg := map[string]interface{}{
			"method": "SUBSCRIBE",
			"params": symbols,
			"id":     1,
		}
		_ = conn.WriteJSON(msg)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				log.Println("ws read:", err)
				close(ch)
				return
			}
			ch <- data
		}
	}()
	return ch, nil
}
