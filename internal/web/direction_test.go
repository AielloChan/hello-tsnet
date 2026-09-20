package web

import (
	"net/http"
	"testing"

	"hello-tsnet/internal/config"
	"hello-tsnet/internal/rules"
)

func TestRulesAPIInboundAndOmittedUpdateDirection(t *testing.T) {
	service := rules.NewService(protocolStore{}, protocolPreparer{})
	handler := NewHandler("127.0.0.1:6060", service, func() Status { return Status{} }, nil)
	created := request(t, handler, http.MethodPost, "/api/rules", []byte(`{"name":"dns","localPort":53,"direction":"inbound"}`), true)
	if created.Code != http.StatusCreated { t.Fatalf("POST = %d %s", created.Code, created.Body.String()) }
	item := decodeJSON[config.Rule](t, created)
	if item.Direction != config.DirectionInbound || item.Protocol != config.ProtocolBoth || item.Target != "" { t.Fatalf("created = %#v", item) }
	path := "/api/rules/" + item.ID
	updated := request(t, handler, http.MethodPut, path, []byte(`{"name":"dns renamed","localPort":53}`), true)
	if updated.Code != http.StatusOK { t.Fatalf("PUT = %d %s", updated.Code, updated.Body.String()) }
	item = decodeJSON[config.Rule](t, updated)
	if item.Direction != config.DirectionInbound || item.Protocol != config.ProtocolBoth { t.Fatalf("omitted PUT = %#v", item) }
	for _, body := range []string{
		`{"name":"dns","localPort":53,"direction":"invalid"}`,
		`{"name":"dns","localPort":53,"direction":"inbound","target":"other:53"}`,
		`{"name":"dns","localPort":0,"direction":"inbound"}`,
	} {
		for _, method := range []string{http.MethodPost, http.MethodPut} {
			url := "/api/rules"
			if method == http.MethodPut { url = path }
			response := request(t, handler, method, url, []byte(body), true)
			if response.Code != http.StatusBadRequest { t.Fatalf("%s = %d %s", method, response.Code, response.Body.String()) }
		}
	}
	conflict := request(t, handler, http.MethodPost, "/api/rules", []byte(`{"name":"duplicate","localPort":53,"direction":"inbound"}`), true)
	if conflict.Code != http.StatusConflict { t.Fatalf("duplicate = %d", conflict.Code) }
}
