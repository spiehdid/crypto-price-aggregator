package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type Config struct {
	WSURL string
}

type miniTicker struct {
	Event  string `json:"e"`
	Symbol string `json:"s"`
	Close  string `json:"c"`
	Time   int64  `json:"E"`
}

type Provider struct {
	cfg    Config
	mapper *SymbolMapper
	mu     sync.Mutex
	conn   *websocket.Conn
	cancel context.CancelFunc
}

func NewProvider(cfg Config) *Provider {
	return &Provider{
		cfg:    cfg,
		mapper: NewSymbolMapper(),
	}
}

func (p *Provider) Name() string { return "binance" }

func (p *Provider) Subscribe(ctx context.Context, coinIDs []string, currency string) (<-chan model.Price, error) {
	// Close existing connection if any
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
	p.mu.Unlock()

	var streams []string
	for _, id := range coinIDs {
		stream, ok := p.mapper.StreamName(id)
		if !ok {
			slog.Warn("binance: unknown coin, skipping", "coin", id)
			continue
		}
		streams = append(streams, stream)
	}
	if len(streams) == 0 {
		return nil, fmt.Errorf("no valid streams for coins: %v", coinIDs)
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, p.cfg.WSURL, nil)
	if err != nil {
		return nil, fmt.Errorf("connecting to binance ws: %w", err)
	}

	p.mu.Lock()
	p.conn = conn
	p.mu.Unlock()

	subMsg := map[string]interface{}{
		"method": "SUBSCRIBE",
		"params": streams,
		"id":     1,
	}
	if err := conn.WriteJSON(subMsg); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("sending subscribe: %w", err)
	}

	ch := make(chan model.Price, 100)
	subCtx, cancel := context.WithCancel(ctx)
	p.mu.Lock()
	p.cancel = cancel
	p.mu.Unlock()

	go p.readLoop(subCtx, conn, ch, currency)
	return ch, nil
}

func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

func (p *Provider) readLoop(ctx context.Context, conn *websocket.Conn, ch chan<- model.Price, currency string) {
	defer close(ch)

	// Close connection when context cancelled — unblocks ReadMessage
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				slog.Warn("binance ws read error", "error", err)
			}
			return
		}

		var ticker miniTicker
		if err := json.Unmarshal(msg, &ticker); err != nil {
			continue
		}
		if ticker.Event != "24hrMiniTicker" {
			continue
		}

		coinID, ok := p.mapper.ToCoinID(strings.ToLower(ticker.Symbol))
		if !ok {
			continue
		}

		price, err := decimal.NewFromString(ticker.Close)
		if err != nil {
			continue
		}

		now := time.Now()
		select {
		case ch <- model.Price{
			CoinID:     coinID,
			Currency:   currency,
			Value:      price,
			Provider:   "binance",
			Timestamp:  time.UnixMilli(ticker.Time),
			ReceivedAt: now,
		}:
		default:
			slog.Debug("binance: price dropped (channel full)", "coin", coinID)
		}
	}
}
