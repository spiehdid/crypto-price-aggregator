package ws

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type clientMessage struct {
	Action   string   `json:"action"`
	Coins    []string `json:"coins"`
	Currency string   `json:"currency"`
}

func HandleWS(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("ws upgrade failed", "error", err)
			return
		}

		send := make(chan []byte, 64)
		var client *Client

		go func() {
			defer func() { _ = conn.Close() }()
			for msg := range send {
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			}
		}()

		defer func() {
			if client != nil {
				hub.RemoveClient(client)
			}
			close(send)
		}()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					slog.Warn("ws read error", "error", err)
				}
				return
			}

			var cm clientMessage
			if err := json.Unmarshal(msg, &cm); err != nil {
				continue
			}

			switch cm.Action {
			case "subscribe":
				if client == nil {
					client = hub.AddClient(send, cm.Coins)
				} else {
					hub.UpdateClientSubscriptions(client, cm.Coins)
				}
			case "unsubscribe":
				if client != nil {
					remaining := make([]string, 0)
					for id := range client.CoinIDs {
						found := false
						for _, c := range cm.Coins {
							if c == id {
								found = true
								break
							}
						}
						if !found {
							remaining = append(remaining, id)
						}
					}
					hub.UpdateClientSubscriptions(client, remaining)
				}
			}
		}
	}
}
