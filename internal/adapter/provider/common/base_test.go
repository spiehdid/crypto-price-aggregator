package common_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spiehdid/crypto-price-aggregator/internal/adapter/provider/common"
	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBaseProvider_Name(t *testing.T) {
	bp := common.NewBaseProvider("testprovider", model.TierFree)
	assert.Equal(t, "testprovider", bp.Name())
}

func TestBaseProvider_StatusInitial(t *testing.T) {
	bp := common.NewBaseProvider("testprovider", model.TierFree)
	status := bp.Status()
	require.NotNil(t, status)
	assert.Equal(t, "testprovider", status.Name)
	assert.True(t, status.Healthy)
	assert.Equal(t, model.TierFree, status.Tier)
	assert.Nil(t, status.LastError)
	assert.True(t, status.LastSuccessAt.IsZero())
}

func TestBaseProvider_RecordSuccess(t *testing.T) {
	bp := common.NewBaseProvider("testprovider", model.TierPaid)

	// First record an error to make it unhealthy.
	bp.RecordError(errors.New("some error"))
	assert.False(t, bp.Status().Healthy)

	// Then record success.
	bp.RecordSuccess()
	status := bp.Status()
	assert.True(t, status.Healthy)
	assert.Nil(t, status.LastError)
	assert.False(t, status.LastSuccessAt.IsZero())
}

func TestBaseProvider_RecordError(t *testing.T) {
	bp := common.NewBaseProvider("testprovider", model.TierFree)
	assert.True(t, bp.Status().Healthy)

	sentinel := errors.New("provider down")
	bp.RecordError(sentinel)

	status := bp.Status()
	assert.False(t, status.Healthy)
	assert.Equal(t, sentinel, status.LastError)
}

func TestBaseProvider_RecordError_DomainError(t *testing.T) {
	bp := common.NewBaseProvider("testprovider", model.TierFree)
	bp.RecordError(model.ErrRateLimited)

	status := bp.Status()
	assert.False(t, status.Healthy)
	assert.ErrorIs(t, status.LastError, model.ErrRateLimited)
}

func TestBaseProvider_TierPaid(t *testing.T) {
	bp := common.NewBaseProvider("premium", model.TierPaid)
	assert.Equal(t, model.TierPaid, bp.Status().Tier)
}

func TestBaseProvider_DoRequest_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-value", r.Header.Get("X-Test"))
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	bp := common.NewBaseProvider("test", model.TierFree)
	resp, err := bp.DoRequest(context.Background(), server.URL, map[string]string{"X-Test": "test-value"})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, 200, resp.StatusCode)
}

func TestBaseProvider_DoRequest_429(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer server.Close()
	bp := common.NewBaseProvider("test", model.TierFree)
	_, err := bp.DoRequest(context.Background(), server.URL, nil)
	assert.ErrorIs(t, err, model.ErrRateLimited)
}

func TestBaseProvider_DoRequest_500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()
	bp := common.NewBaseProvider("test", model.TierFree)
	_, err := bp.DoRequest(context.Background(), server.URL, nil)
	assert.ErrorIs(t, err, model.ErrProviderDown)
}

func TestBaseProvider_DoRequest_401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer server.Close()
	bp := common.NewBaseProvider("test", model.TierFree)
	_, err := bp.DoRequest(context.Background(), server.URL, nil)
	assert.ErrorIs(t, err, model.ErrUnauthorized)
}

func TestBaseProvider_DoRequest_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer server.Close()
	bp := common.NewBaseProvider("test", model.TierFree)
	_, err := bp.DoRequest(context.Background(), server.URL, nil)
	assert.ErrorIs(t, err, model.ErrCoinNotFound)
}

func TestBaseProvider_DecodeJSON(t *testing.T) {
	body := strings.NewReader(`{"price":"67432.15"}`)
	var result map[string]json.Number
	bp := common.NewBaseProvider("test", model.TierFree)
	err := bp.DecodeJSON(body, &result)
	require.NoError(t, err)
	assert.Equal(t, json.Number("67432.15"), result["price"])
}

func TestBaseProvider_DoRequest_UnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(302)
	}))
	defer server.Close()
	bp := common.NewBaseProvider("test", model.TierFree)
	_, err := bp.DoRequest(context.Background(), server.URL, nil)
	assert.Error(t, err)
}

func TestBaseProvider_DecodeJSON_Invalid(t *testing.T) {
	body := strings.NewReader(`{invalid`)
	var result map[string]interface{}
	bp := common.NewBaseProvider("test", model.TierFree)
	err := bp.DecodeJSON(body, &result)
	assert.Error(t, err)
}
