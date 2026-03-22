package app

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type AlertService struct {
	mu           sync.RWMutex
	alerts       map[string]*model.Alert
	client       *http.Client
	urlValidator func(string) error
	wg           sync.WaitGroup
}

func NewAlertService() *AlertService {
	return &AlertService{
		alerts:       make(map[string]*model.Alert),
		client:       &http.Client{Timeout: 5 * time.Second},
		urlValidator: validateWebhookURL,
	}
}

func (s *AlertService) Stop() {
	s.wg.Wait()
}

func (s *AlertService) OnPriceUpdate(price *model.Price) {
	s.mu.RLock()
	var triggered []*model.Alert
	for _, alert := range s.alerts {
		if !alert.Active {
			continue
		}
		if alert.CoinID != price.CoinID || alert.Currency != price.Currency {
			continue
		}
		if s.conditionMet(alert, price) {
			triggered = append(triggered, alert)
		}
	}
	s.mu.RUnlock()

	for _, alert := range triggered {
		s.trigger(alert, price)
	}
}

func (s *AlertService) conditionMet(alert *model.Alert, price *model.Price) bool {
	switch alert.Condition {
	case model.ConditionAbove:
		return price.Value.GreaterThanOrEqual(alert.Threshold)
	case model.ConditionBelow:
		return price.Value.LessThanOrEqual(alert.Threshold)
	}
	return false
}

func (s *AlertService) trigger(alert *model.Alert, price *model.Price) {
	s.mu.Lock()
	if !alert.Active {
		s.mu.Unlock()
		return // already triggered by another goroutine
	}
	now := time.Now()
	alert.Active = false
	alert.TriggeredAt = &now
	s.mu.Unlock()

	slog.Info("alert triggered", "id", alert.ID, "coin", alert.CoinID, "condition", alert.Condition.String(), "price", price.Value.String())

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.fireWebhook(alert, price)
	}()
}

type webhookPayload struct {
	AlertID     string `json:"alert_id"`
	Coin        string `json:"coin"`
	Currency    string `json:"currency"`
	Condition   string `json:"condition"`
	Threshold   string `json:"threshold"`
	Price       string `json:"price"`
	TriggeredAt string `json:"triggered_at"`
}

func (s *AlertService) fireWebhook(alert *model.Alert, price *model.Price) {
	payload := webhookPayload{
		AlertID:     alert.ID,
		Coin:        alert.CoinID,
		Currency:    alert.Currency,
		Condition:   alert.Condition.String(),
		Threshold:   alert.Threshold.String(),
		Price:       price.Value.String(),
		TriggeredAt: alert.TriggeredAt.Format(time.RFC3339),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("alert webhook marshal failed", "id", alert.ID, "error", err)
		return
	}

	for attempt := 0; attempt < 3; attempt++ {
		resp, err := s.client.Post(alert.WebhookURL, "application/json", bytes.NewReader(data))
		if err == nil {
			if resp.StatusCode < 500 {
				_ = resp.Body.Close()
				slog.Info("alert webhook delivered", "id", alert.ID, "status", resp.StatusCode)
				return
			}
			_ = resp.Body.Close()
		}
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}
	slog.Error("alert webhook failed after 3 attempts", "id", alert.ID, "url", alert.WebhookURL)
}

func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https schemes allowed")
	}

	host := u.Hostname()

	// Block private/internal IPs
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("internal IP addresses not allowed: %s", host)
		}
	}

	// Block known internal hostnames
	lowerHost := strings.ToLower(host)
	blocked := []string{"localhost", "metadata.google", "169.254.169.254"}
	for _, b := range blocked {
		if strings.Contains(lowerHost, b) {
			return fmt.Errorf("blocked hostname: %s", host)
		}
	}

	return nil
}

func generateAlertID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "alert_" + hex.EncodeToString(b)
}

func (s *AlertService) Create(alert *model.Alert) error {
	if err := s.urlValidator(alert.WebhookURL); err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	if alert.ID == "" {
		alert.ID = generateAlertID()
	}
	alert.Active = true
	alert.CreatedAt = time.Now()

	s.mu.Lock()
	s.alerts[alert.ID] = alert
	s.mu.Unlock()
	return nil
}

func (s *AlertService) List() []*model.Alert {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*model.Alert, 0, len(s.alerts))
	for _, a := range s.alerts {
		copy := *a
		result = append(result, &copy)
	}
	return result
}

func (s *AlertService) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.alerts[id]; !ok {
		return fmt.Errorf("alert not found: %s", id)
	}
	delete(s.alerts, id)
	return nil
}

// compile-time check: AlertService implements PriceListener
var _ PriceListener = (*AlertService)(nil)
