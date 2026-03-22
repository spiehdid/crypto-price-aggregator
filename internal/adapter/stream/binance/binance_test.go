package binance_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/stream/binance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockBinanceServer(t *testing.T) *httptest.Server {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()

		_, _, _ = conn.ReadMessage() // consume subscribe
		_ = conn.WriteJSON(map[string]interface{}{"result": nil, "id": 1})

		for i := 0; i < 3; i++ {
			_ = conn.WriteJSON(map[string]interface{}{
				"e": "24hrMiniTicker",
				"s": "BTCUSDT",
				"c": "67432.15",
				"o": "66500.00",
				"h": "68000.00",
				"l": "66000.00",
				"E": time.Now().UnixMilli(),
			})
			time.Sleep(50 * time.Millisecond)
		}
	}))
	return server
}

func TestProvider_Subscribe_ReceivesPrices(t *testing.T) {
	server := newMockBinanceServer(t)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	provider := binance.NewProvider(binance.Config{WSURL: wsURL})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := provider.Subscribe(ctx, []string{"bitcoin"}, "usd")
	require.NoError(t, err)

	var received int
	for price := range ch {
		assert.Equal(t, "bitcoin", price.CoinID)
		assert.Equal(t, "binance", price.Provider)
		received++
		if received >= 2 {
			break
		}
	}
	assert.GreaterOrEqual(t, received, 2)
	_ = provider.Close()
}

func TestProvider_Name(t *testing.T) {
	provider := binance.NewProvider(binance.Config{WSURL: "ws://localhost"})
	assert.Equal(t, "binance", provider.Name())
}
