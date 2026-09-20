package rules

import (
	"errors"
	"testing"

	"hello-tsnet/internal/config"
)

func TestInboundUpdatePreservesOmittedDirectionAndProtocol(t *testing.T) {
	service := NewService(&fakeStore{}, &fakePreparer{})
	added, err := service.Add("dns", 53, "", config.ProtocolUDP, config.DirectionInbound)
	if err != nil { t.Fatal(err) }
	updated, err := service.Update(added.ID, "dns renamed", 53, "", "", "")
	if err != nil || updated.Direction != config.DirectionInbound || updated.Protocol != config.ProtocolUDP {
		t.Fatalf("Update() = %#v, %v", updated, err)
	}
	if _, err := service.Update(added.ID, "dns", 53, "evil:53", "", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("inbound with target error = %v", err)
	}
	if _, err := service.Add("bad", 8080, "", config.ProtocolTCP, "invalid"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid direction error = %v", err)
	}
}

func TestInboundSaveFailurePreservesOutboundRule(t *testing.T) {
	store := &fakeStore{}
	preparer := &fakePreparer{}
	service := NewService(store, preparer)
	added, err := service.Add("web", 8080, "host:80", config.ProtocolBoth, config.DirectionOutbound)
	if err != nil { t.Fatal(err) }
	store.saveErr = errors.New("disk full")
	if _, err := service.Update(added.ID, "web", 8080, "", config.ProtocolBoth, config.DirectionInbound); err == nil { t.Fatal("Update should fail") }
	if service.List()[0] != added || preparer.change.aborts != 1 || preparer.change.commits != 0 { t.Fatal("failed inbound change affected existing rule") }
}
