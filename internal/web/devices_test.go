package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hello-tsnet/internal/tailnet"
)

func TestDevicesAPIIsNoStoreAndBounded(t *testing.T) {
	called := false
	reader := func(ctx context.Context) ([]tailnet.Device, error) {
		called = true
		if _, ok := ctx.Deadline(); !ok { t.Error("device lookup has no deadline") }
		return []tailnet.Device{{ID: "one", Name: "Mac mini", State: "online"}}, nil
	}
	handler := NewHandler("127.0.0.1:6060", &fakeRuleService{}, func() Status { return Status{} }, reader)
	response := request(t, handler, http.MethodGet, "/api/devices", nil, false)
	if !called || response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), "Mac mini") {
		t.Fatalf("devices = %d %s", response.Code, response.Body.String())
	}
	called = false
	req := httptest.NewRequest(http.MethodGet, "http://evil.example/api/devices", nil)
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, req)
	if denied.Code != 403 || called { t.Fatal("foreign Host accessed devices") }
}

func TestDevicesAPIDistinguishesFailureFromEmptyList(t *testing.T) {
	for _, failed := range []bool{false, true} {
		reader := func(context.Context) ([]tailnet.Device, error) {
			if failed { return nil, errors.New("sensitive backend error") }
			return nil, nil
		}
		handler := NewHandler("127.0.0.1:6060", &fakeRuleService{}, func() Status { return Status{} }, reader)
		response := request(t, handler, http.MethodGet, "/api/devices", nil, false)
		if failed {
			if response.Code != 503 || strings.Contains(response.Body.String(), "sensitive") { t.Fatalf("failure = %d %s", response.Code, response.Body.String()) }
		} else if response.Code != 200 || strings.TrimSpace(response.Body.String()) != "[]" { t.Fatalf("empty = %d %s", response.Code, response.Body.String()) }
	}
}
