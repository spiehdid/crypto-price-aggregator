package ws_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/transport/ws"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHub_BroadcastToSubscribedClients(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()

	ch := make(chan []byte, 10)
	client := hub.AddClient(ch, []string{"bitcoin"})

	price := &model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(67000), Provider: "test",
		Timestamp: time.Now(), ReceivedAt: time.Now(),
	}
	hub.OnPriceUpdate(price)

	select {
	case msg := <-ch:
		assert.Contains(t, string(msg), "bitcoin")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected message but got none")
	}
	hub.RemoveClient(client)
}

func TestHub_DoesNotSendUnsubscribedCoins(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()

	ch := make(chan []byte, 10)
	hub.AddClient(ch, []string{"ethereum"})

	price := &model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(67000), Provider: "test",
	}
	hub.OnPriceUpdate(price)

	select {
	case <-ch:
		t.Fatal("should not receive bitcoin updates")
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

// TestHub_RunBroadcastToMultipleClients verifies that Run dispatches a price
// update to every subscribed client concurrently.
func TestHub_RunBroadcastToMultipleClients(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()

	ch1 := make(chan []byte, 10)
	ch2 := make(chan []byte, 10)
	hub.AddClient(ch1, []string{"bitcoin"})
	hub.AddClient(ch2, []string{"bitcoin"})

	price := &model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(50000), Provider: "test",
		Timestamp: time.Now(), ReceivedAt: time.Now(),
	}
	hub.OnPriceUpdate(price)

	for i, ch := range []chan []byte{ch1, ch2} {
		select {
		case msg := <-ch:
			assert.Contains(t, string(msg), "bitcoin", "client %d should receive bitcoin update", i+1)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("client %d: expected message but got none", i+1)
		}
	}
}

// TestHub_RunBroadcastMessageFields verifies that broadcast JSON contains all
// expected fields with correct values.
func TestHub_RunBroadcastMessageFields(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()

	ch := make(chan []byte, 10)
	hub.AddClient(ch, []string{"ethereum"})

	ts := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	price := &model.Price{
		CoinID:    "ethereum",
		Currency:  "usd",
		Value:     decimal.NewFromFloat(3500.50),
		Provider:  "coingecko",
		Timestamp: ts,
	}
	hub.OnPriceUpdate(price)

	select {
	case msg := <-ch:
		var m map[string]string
		require.NoError(t, json.Unmarshal(msg, &m))
		assert.Equal(t, "ethereum", m["coin"])
		assert.Equal(t, "usd", m["currency"])
		assert.Equal(t, "coingecko", m["provider"])
		assert.Equal(t, "2024-06-01T12:00:00Z", m["timestamp"])
		assert.Contains(t, m["price"], "3500")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected message but got none")
	}
}

// TestHub_UpdateClientSubscriptions verifies that after updating subscriptions
// the client receives only the newly subscribed coin.
func TestHub_UpdateClientSubscriptions(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()

	ch := make(chan []byte, 10)
	client := hub.AddClient(ch, []string{"bitcoin"})

	// Switch subscription from bitcoin to ethereum.
	hub.UpdateClientSubscriptions(client, []string{"ethereum"})

	bitcoinPrice := &model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(60000), Provider: "test",
		Timestamp: time.Now(),
	}
	hub.OnPriceUpdate(bitcoinPrice)

	// Should NOT receive bitcoin.
	select {
	case msg := <-ch:
		t.Fatalf("should not receive bitcoin after re-subscription, got: %s", msg)
	case <-time.After(50 * time.Millisecond):
		// good
	}

	ethereumPrice := &model.Price{
		CoinID: "ethereum", Currency: "usd",
		Value: decimal.NewFromFloat(3000), Provider: "test",
		Timestamp: time.Now(),
	}
	hub.OnPriceUpdate(ethereumPrice)

	// Should receive ethereum.
	select {
	case msg := <-ch:
		assert.Contains(t, string(msg), "ethereum")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected ethereum message after subscription update")
	}
}

// TestHub_RemoveClientStopsDelivery verifies that after RemoveClient the
// removed client receives no further messages.
func TestHub_RemoveClientStopsDelivery(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()

	ch := make(chan []byte, 10)
	client := hub.AddClient(ch, []string{"bitcoin"})
	hub.RemoveClient(client)

	price := &model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(70000), Provider: "test",
		Timestamp: time.Now(),
	}
	hub.OnPriceUpdate(price)

	select {
	case msg := <-ch:
		t.Fatalf("removed client should not receive messages, got: %s", msg)
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

// TestHub_OnPriceUpdateFullChannelDropsBehavior verifies that OnPriceUpdate
// does not block when the internal updates channel is full.
func TestHub_OnPriceUpdateFullChannelDropsBehavior(t *testing.T) {
	hub := ws.NewHub()
	// Do NOT call hub.Run() so nothing drains the channel.
	// Fill the channel (capacity 256) then one more; must not block/panic.
	price := &model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(1), Provider: "test",
	}

	done := make(chan struct{})
	go func() {
		for i := 0; i < 300; i++ {
			hub.OnPriceUpdate(price)
		}
		close(done)
	}()

	select {
	case <-done:
		// completed without blocking
	case <-time.After(500 * time.Millisecond):
		t.Fatal("OnPriceUpdate blocked on full channel")
	}
}

// TestHub_MultipleCoinsFiltering verifies that a client subscribed to several
// coins receives only matching updates.
func TestHub_MultipleCoinsFiltering(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()

	ch := make(chan []byte, 20)
	hub.AddClient(ch, []string{"bitcoin", "ethereum"})

	prices := []*model.Price{
		{CoinID: "bitcoin", Currency: "usd", Value: decimal.NewFromFloat(60000), Provider: "test", Timestamp: time.Now()},
		{CoinID: "ethereum", Currency: "usd", Value: decimal.NewFromFloat(3000), Provider: "test", Timestamp: time.Now()},
		{CoinID: "solana", Currency: "usd", Value: decimal.NewFromFloat(150), Provider: "test", Timestamp: time.Now()},
	}
	for _, p := range prices {
		hub.OnPriceUpdate(p)
	}

	// Collect received messages over a short window.
	received := make(map[string]bool)
	deadline := time.After(150 * time.Millisecond)
collect:
	for {
		select {
		case msg := <-ch:
			s := string(msg)
			switch {
			case strings.Contains(s, "bitcoin"):
				received["bitcoin"] = true
			case strings.Contains(s, "ethereum"):
				received["ethereum"] = true
			case strings.Contains(s, "solana"):
				received["solana"] = true
			}
		case <-deadline:
			break collect
		}
	}

	assert.True(t, received["bitcoin"], "should receive bitcoin")
	assert.True(t, received["ethereum"], "should receive ethereum")
	assert.False(t, received["solana"], "should NOT receive solana")
}
