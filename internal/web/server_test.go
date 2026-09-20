package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hello-tsnet/internal/config"
	"hello-tsnet/internal/rules"
)

type fakeRuleService struct {
	items       []config.Rule
	addErr      error
	updateErr   error
	deleteErr   error
	lastUpdated string
	lastDeleted string
}

func (s *fakeRuleService) List() []config.Rule {
	return append([]config.Rule(nil), s.items...)
}

func (s *fakeRuleService) Add(name string, port int, target string, protocol config.Protocol, direction config.Direction) (config.Rule, error) {
	if s.addErr != nil {
		return config.Rule{}, s.addErr
	}
	rule, err := config.NewRule(name, port, target, protocol, direction)
	if err != nil { return config.Rule{}, fmt.Errorf("%w: %v", rules.ErrInvalid, err) }
	rule.ID = "new-id"
	s.items = append(s.items, rule)
	return rule, nil
}

func (s *fakeRuleService) Update(id, name string, port int, target string, protocol config.Protocol, direction config.Direction) (config.Rule, error) {
	if s.updateErr != nil {
		return config.Rule{}, s.updateErr
	}
	s.lastUpdated = id
	return config.Rule{ID: id, Name: name, LocalPort: port, Target: target, Protocol: protocol, Direction: direction}, nil
}

func (s *fakeRuleService) Delete(id string) error {
	s.lastDeleted = id
	return s.deleteErr
}

func TestRulesAPIListsAndCreatesRules(t *testing.T) {
	service := &fakeRuleService{items: []config.Rule{{ID: "one", Name: "ssh", LocalPort: 30001, Target: "host:22"}}}
	handler := NewHandler("127.0.0.1:6060", service, func() Status { return Status{State: StatePending} }, nil)

	list := request(t, handler, http.MethodGet, "/api/rules", nil, false)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "host:22") {
		t.Fatalf("GET /api/rules = %d %s", list.Code, list.Body.String())
	}

	create := request(t, handler, http.MethodPost, "/api/rules", []byte(`{"name":"screen","localPort":30002,"target":"mac-mini:5900"}`), true)
	if create.Code != http.StatusCreated || !strings.Contains(create.Body.String(), "new-id") {
		t.Fatalf("POST /api/rules = %d %s", create.Code, create.Body.String())
	}
}

func TestRulesAPIEncodesEmptyListAsArray(t *testing.T) {
	handler := NewHandler("127.0.0.1:6060", &fakeRuleService{}, func() Status { return Status{} }, nil)
	response := request(t, handler, http.MethodGet, "/api/rules", nil, false)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("GET empty /api/rules = %d %q, want []", response.Code, response.Body.String())
	}
}

func TestRulesAPIUpdatesAndDeletesRules(t *testing.T) {
	service := &fakeRuleService{}
	handler := NewHandler("127.0.0.1:6060", service, func() Status { return Status{State: StateReady} }, nil)

	update := request(t, handler, http.MethodPut, "/api/rules/rule-1", []byte(`{"name":"vnc","localPort":30001,"target":"mac-mini:5900"}`), true)
	if update.Code != http.StatusOK || service.lastUpdated != "rule-1" {
		t.Fatalf("PUT = %d, updated = %q", update.Code, service.lastUpdated)
	}
	remove := request(t, handler, http.MethodDelete, "/api/rules/rule-1", nil, true)
	if remove.Code != http.StatusNoContent || service.lastDeleted != "rule-1" {
		t.Fatalf("DELETE = %d, deleted = %q", remove.Code, service.lastDeleted)
	}
}

func TestDeleteAllowsAnEmptyBodyWithoutJSONContentType(t *testing.T) {
	service := &fakeRuleService{}
	handler := NewHandler("127.0.0.1:6060", service, func() Status { return Status{} }, nil)
	req := httptest.NewRequest(http.MethodDelete, "http://127.0.0.1:6060/api/rules/rule-1", nil)
	req.Host = "127.0.0.1:6060"
	req.Header.Set("Origin", "http://127.0.0.1:6060")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusNoContent || service.lastDeleted != "rule-1" {
		t.Fatalf("DELETE without body = %d, deleted = %q", response.Code, service.lastDeleted)
	}
}

func TestRulesAPIReturnsNotFoundAndValidationErrors(t *testing.T) {
	service := &fakeRuleService{updateErr: rules.ErrNotFound, deleteErr: rules.ErrNotFound}
	handler := NewHandler("127.0.0.1:6060", service, func() Status { return Status{} }, nil)

	invalid := request(t, handler, http.MethodPost, "/api/rules", []byte(`{"name":"","localPort":22,"target":"bad"}`), true)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid POST status = %d", invalid.Code)
	}
	missing := request(t, handler, http.MethodPut, "/api/rules/missing", []byte(`{"name":"ssh","localPort":30001,"target":"host:22"}`), true)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing PUT status = %d", missing.Code)
	}
	deleted := request(t, handler, http.MethodDelete, "/api/rules/missing", nil, true)
	if deleted.Code != http.StatusNotFound {
		t.Fatalf("missing DELETE status = %d", deleted.Code)
	}
}

func TestWriteRequestsRequireExactLocalOriginAndJSON(t *testing.T) {
	handler := NewHandler("127.0.0.1:6060", &fakeRuleService{}, func() Status { return Status{} }, nil)
	body := []byte(`{"name":"ssh","localPort":30001,"target":"host:22"}`)

	tests := []struct {
		name        string
		host        string
		origin      string
		contentType string
		method      string
		want        int
	}{
		{name: "foreign host", host: "evil.example", origin: "http://127.0.0.1:6060", contentType: "application/json", method: http.MethodPost, want: http.StatusForbidden},
		{name: "missing origin", host: "127.0.0.1:6060", contentType: "application/json", method: http.MethodPost, want: http.StatusForbidden},
		{name: "foreign origin", host: "127.0.0.1:6060", origin: "https://evil.example", contentType: "application/json", method: http.MethodPost, want: http.StatusForbidden},
		{name: "wrong content type", host: "127.0.0.1:6060", origin: "http://127.0.0.1:6060", contentType: "text/plain", method: http.MethodPost, want: http.StatusUnsupportedMediaType},
		{name: "cors preflight", host: "127.0.0.1:6060", origin: "http://127.0.0.1:6060", method: http.MethodOptions, want: http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://127.0.0.1:6060/api/rules", bytes.NewReader(body))
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d", response.Code, tt.want)
			}
			if response.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("response unexpectedly enabled CORS")
			}
		})
	}
}

func TestWriteRequestBodyIsLimited(t *testing.T) {
	handler := NewHandler("127.0.0.1:6060", &fakeRuleService{}, func() Status { return Status{} }, nil)
	large := []byte(`{"name":"` + strings.Repeat("x", 70<<10) + `","localPort":30001,"target":"host:22"}`)
	response := request(t, handler, http.MethodPost, "/api/rules", large, true)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large POST status = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestStatusIsNoStoreAndHidesAuthURLWhenReady(t *testing.T) {
	status := Status{State: StatePending, AuthURL: "https://login.tailscale.com/a/example"}
	handler := NewHandler("127.0.0.1:6060", &fakeRuleService{}, func() Status { return status }, nil)

	pending := request(t, handler, http.MethodGet, "/api/status", nil, false)
	if pending.Header().Get("Cache-Control") != "no-store" || !strings.Contains(pending.Body.String(), status.AuthURL) {
		t.Fatalf("pending status headers/body = %#v %s", pending.Header(), pending.Body.String())
	}
	status.State = StateReady
	ready := request(t, handler, http.MethodGet, "/api/status", nil, false)
	if ready.Header().Get("Cache-Control") != "no-store" || strings.Contains(ready.Body.String(), "login.tailscale.com") {
		t.Fatalf("ready status headers/body = %#v %s", ready.Header(), ready.Body.String())
	}
}

func TestEmbeddedIndexIsServed(t *testing.T) {
	handler := NewHandler("127.0.0.1:6060", &fakeRuleService{}, func() Status { return Status{} }, nil)
	response := request(t, handler, http.MethodGet, "/", nil, false)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Hello TSNet") || !strings.Contains(response.Body.String(), `id="close-dialog" type="button"`) {
		t.Fatalf("GET / = %d %s", response.Code, response.Body.String())
	}
}

func request(t *testing.T, handler http.Handler, method, path string, body []byte, write bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "http://127.0.0.1:6060"+path, bytes.NewReader(body))
	req.Host = "127.0.0.1:6060"
	if write {
		req.Header.Set("Origin", "http://127.0.0.1:6060")
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func decodeJSON[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return value
}

var _ = errors.Is
