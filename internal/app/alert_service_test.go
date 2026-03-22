package app_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spiehdid/crypto-price-aggregator/internal/app"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validWebhookURL = "https://hooks.example.com/webhook"

func TestAlertService_CreateAndList(t *testing.T) {
	svc := app.NewAlertService()

	alert := &model.Alert{
		CoinID:     "bitcoin",
		Currency:   "usd",
		Condition:  model.ConditionAbove,
		Threshold:  decimal.NewFromFloat(70000),
		WebhookURL: validWebhookURL,
	}

	err := svc.Create(alert)
	require.NoError(t, err)
	assert.NotEmpty(t, alert.ID)
	assert.True(t, alert.Active)

	alerts := svc.List()
	assert.Len(t, alerts, 1)
}

func TestAlertService_Delete(t *testing.T) {
	svc := app.NewAlertService()
	alert := &model.Alert{
		CoinID: "bitcoin", Currency: "usd",
		Condition: model.ConditionAbove, Threshold: decimal.NewFromFloat(70000),
		WebhookURL: validWebhookURL,
	}
	err := svc.Create(alert)
	require.NoError(t, err)

	err = svc.Delete(alert.ID)
	require.NoError(t, err)
	assert.Len(t, svc.List(), 0)
}

func TestAlertService_TriggersOnPriceUpdate(t *testing.T) {
	var webhookCalled atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookCalled.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	svc := app.NewAlertService()
	// Allow the loopback httptest URL for this test only
	svc.SetURLValidator(func(string) error { return nil })

	alert := &model.Alert{
		CoinID: "bitcoin", Currency: "usd",
		Condition: model.ConditionAbove, Threshold: decimal.NewFromFloat(70000),
		WebhookURL: server.URL,
	}
	err := svc.Create(alert)
	require.NoError(t, err)

	// Price below threshold — should not trigger
	svc.OnPriceUpdate(&model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(69000),
	})
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), webhookCalled.Load())

	// Price above threshold — should trigger
	svc.OnPriceUpdate(&model.Price{
		CoinID: "bitcoin", Currency: "usd",
		Value: decimal.NewFromFloat(70500),
	})
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(1), webhookCalled.Load())

	// Alert should be deactivated (one-shot)
	assert.False(t, svc.List()[0].Active)
}

func TestAlertService_DoesNotTriggerForWrongCoin(t *testing.T) {
	svc := app.NewAlertService()
	alert := &model.Alert{
		CoinID: "bitcoin", Currency: "usd",
		Condition: model.ConditionAbove, Threshold: decimal.NewFromFloat(70000),
		WebhookURL: validWebhookURL,
	}
	err := svc.Create(alert)
	require.NoError(t, err)

	svc.OnPriceUpdate(&model.Price{
		CoinID: "ethereum", Currency: "usd",
		Value: decimal.NewFromFloat(100000),
	})

	assert.True(t, svc.List()[0].Active) // still active, not triggered
}

func TestValidateWebhookURL_Blocks(t *testing.T) {
	tests := []string{
		"http://localhost/hook",
		"http://127.0.0.1/hook",
		"http://10.0.0.1/hook",
		"http://192.168.1.1/hook",
		"http://169.254.169.254/metadata",
		"ftp://example.com/hook",
		"javascript:alert(1)",
	}
	for _, u := range tests {
		svc := app.NewAlertService()
		err := svc.Create(&model.Alert{
			CoinID: "btc", Currency: "usd",
			Condition: model.ConditionAbove, Threshold: decimal.NewFromFloat(70000),
			WebhookURL: u,
		})
		assert.Error(t, err, "should block: %s", u)
	}
}

func TestValidateWebhookURL_Allows(t *testing.T) {
	svc := app.NewAlertService()
	err := svc.Create(&model.Alert{
		CoinID: "btc", Currency: "usd",
		Condition: model.ConditionAbove, Threshold: decimal.NewFromFloat(70000),
		WebhookURL: "https://hooks.example.com/webhook",
	})
	assert.NoError(t, err)
}
