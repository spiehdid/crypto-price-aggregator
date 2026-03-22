package ws

import (
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type Client struct {
	ID      uint64
	Send    chan<- []byte
	CoinIDs map[string]bool
}

type priceMessage struct {
	Coin      string `json:"coin"`
	Currency  string `json:"currency"`
	Price     string `json:"price"`
	Provider  string `json:"provider"`
	Timestamp string `json:"timestamp"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[uint64]*Client
	updates chan *model.Price
	nextID  atomic.Uint64
	done    chan struct{}
	stopped sync.Once
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[uint64]*Client),
		updates: make(chan *model.Price, 256),
		done:    make(chan struct{}),
	}
}

func (h *Hub) Stop() {
	h.stopped.Do(func() { close(h.done) })
}

func (h *Hub) Run() {
	for {
		select {
		case price, ok := <-h.updates:
			if !ok {
				return
			}
			h.broadcast(price)
		case <-h.done:
			return
		}
	}
}

func (h *Hub) OnPriceUpdate(price *model.Price) {
	select {
	case h.updates <- price:
	case <-h.done:
	default:
		slog.Debug("ws hub: price update dropped (channel full)", "coin", price.CoinID)
	}
}

func (h *Hub) AddClient(send chan<- []byte, coinIDs []string) *Client {
	coins := make(map[string]bool, len(coinIDs))
	for _, id := range coinIDs {
		coins[id] = true
	}
	client := &Client{
		ID:      h.nextID.Add(1),
		Send:    send,
		CoinIDs: coins,
	}
	h.mu.Lock()
	h.clients[client.ID] = client
	h.mu.Unlock()
	return client
}

func (h *Hub) RemoveClient(client *Client) {
	h.mu.Lock()
	delete(h.clients, client.ID)
	h.mu.Unlock()
}

func (h *Hub) UpdateClientSubscriptions(client *Client, coinIDs []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	coins := make(map[string]bool, len(coinIDs))
	for _, id := range coinIDs {
		coins[id] = true
	}
	client.CoinIDs = coins
}

func (h *Hub) broadcast(price *model.Price) {
	msg := priceMessage{
		Coin:      price.CoinID,
		Currency:  price.Currency,
		Price:     price.Value.String(),
		Provider:  price.Provider,
		Timestamp: price.Timestamp.Format("2006-01-02T15:04:05Z"),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, client := range h.clients {
		if !client.CoinIDs[price.CoinID] {
			continue
		}
		select {
		case client.Send <- data:
		default:
		}
	}
}
