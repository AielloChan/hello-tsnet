package web

import (
	"fmt"
	"net/http"
	"testing"

	"hello-tsnet/internal/config"
	"hello-tsnet/internal/rules"
)

type protocolStore struct{}
func (protocolStore) Save([]config.Rule) error { return nil }

type protocolPreparer struct{}
func (protocolPreparer) Prepare([]config.Rule) (rules.Change, error) { return protocolChange{}, nil }

type protocolChange struct{}
func (protocolChange) Commit() {}
func (protocolChange) Abort() {}

func TestRulesAPIProtocolCRUD(t *testing.T) {
	service := rules.NewService(protocolStore{}, protocolPreparer{})
	handler := NewHandler("127.0.0.1:6060", service, func() Status { return Status{} }, nil)
	created := request(t, handler, http.MethodPost, "/api/rules", []byte(`{"name":"dns","localPort":30001,"target":"host:53"}`), true)
	if created.Code != http.StatusCreated { t.Fatalf("POST = %d %s", created.Code, created.Body.String()) }
	item := decodeJSON[config.Rule](t, created)
	if item.Protocol != config.ProtocolBoth { t.Fatalf("POST default = %q", item.Protocol) }
	path := "/api/rules/" + item.ID
	for _, protocol := range []config.Protocol{config.ProtocolUDP, config.ProtocolTCP, config.ProtocolBoth} {
		body := []byte(fmt.Sprintf(`{"name":"dns","localPort":30001,"target":"host:53","protocol":%q}`, protocol))
		updated := request(t, handler, http.MethodPut, path, body, true)
		if updated.Code != http.StatusOK || decodeJSON[config.Rule](t, updated).Protocol != protocol {
			t.Fatalf("PUT %q = %d %s", protocol, updated.Code, updated.Body.String())
		}
		omitted := request(t, handler, http.MethodPut, path, []byte(`{"name":"dns","localPort":30001,"target":"other:53"}`), true)
		if omitted.Code != http.StatusOK || decodeJSON[config.Rule](t, omitted).Protocol != protocol {
			t.Fatalf("PUT omitted changed %q: %d %s", protocol, omitted.Code, omitted.Body.String())
		}
		listed := request(t, handler, http.MethodGet, "/api/rules", nil, false)
		items := decodeJSON[[]config.Rule](t, listed)
		if len(items) != 1 || items[0].Protocol != protocol { t.Fatalf("GET = %#v", items) }
	}
	deleted := request(t, handler, http.MethodDelete, path, nil, true)
	if deleted.Code != http.StatusNoContent || len(service.List()) != 0 { t.Fatalf("DELETE = %d", deleted.Code) }
}

func TestRulesAPIRejectsUnknownProtocol(t *testing.T) {
	service := rules.NewService(protocolStore{}, protocolPreparer{})
	handler := NewHandler("127.0.0.1:6060", service, func() Status { return Status{} }, nil)
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		path := "/api/rules"
		if method == http.MethodPut {
			item, err := service.Add("dns", 30001, "host:53", config.ProtocolTCP, config.DirectionOutbound)
			if err != nil { t.Fatal(err) }
			path += "/" + item.ID
		}
		response := request(t, handler, method, path, []byte(`{"name":"dns","localPort":30001,"target":"host:53","protocol":"sctp"}`), true)
		if response.Code != http.StatusBadRequest { t.Fatalf("%s invalid = %d", method, response.Code) }
	}
}
