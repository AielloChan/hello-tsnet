package rules

import (
	"errors"
	"reflect"
	"testing"

	"hello-tsnet/internal/config"
)

func TestAddDefaultsBothAndUpdatePreservesOmittedProtocol(t *testing.T) {
	service := NewService(&fakeStore{}, &fakePreparer{})
	added, err := service.Add("dns", 30001, "host:53", "", "")
	if err != nil || added.Protocol != config.ProtocolBoth { t.Fatalf("Add() = %#v, %v", added, err) }
	updated, err := service.Update(added.ID, "dns", 30001, "other:53", config.ProtocolUDP, config.DirectionOutbound)
	if err != nil || updated.Protocol != config.ProtocolUDP { t.Fatalf("Update() = %#v, %v", updated, err) }
	updated, err = service.Update(added.ID, "dns", 30001, "another:53", "", "")
	if err != nil || updated.Protocol != config.ProtocolUDP { t.Fatalf("omitted Update() = %#v, %v", updated, err) }
	legacy := []config.Rule{{ID: "legacy", Name: "ssh", LocalPort: 30002, Target: "host:22"}}
	if err := service.Restore(legacy); err != nil { t.Fatal(err) }
	updated, err = service.Update("legacy", "ssh", 30002, "other:22", "", "")
	if err != nil || updated.Protocol != config.ProtocolTCP { t.Fatalf("legacy Update() = %#v, %v", updated, err) }
}

func TestUDPUpdateSaveFailurePreservesPreviousProtocol(t *testing.T) {
	store := &fakeStore{}
	preparer := &fakePreparer{}
	service := NewService(store, preparer)
	added, err := service.Add("dns", 30001, "host:53", config.ProtocolTCP, config.DirectionOutbound)
	if err != nil { t.Fatal(err) }
	store.saveErr = errors.New("disk full")
	if _, err := service.Update(added.ID, added.Name, added.LocalPort, added.Target, config.ProtocolBoth, config.DirectionOutbound); err == nil {
		t.Fatal("Update() should fail")
	}
	if preparer.change.aborts != 1 || preparer.change.commits != 0 { t.Fatalf("change = %#v", preparer.change) }
	if !reflect.DeepEqual(service.List(), []config.Rule{added}) || !reflect.DeepEqual(store.saved, []config.Rule{added}) {
		t.Fatalf("failed UDP addition changed previous rule: %#v", service.List())
	}
}

func TestProtocolsDoNotPermitDuplicateLocalPorts(t *testing.T) {
	service := NewService(&fakeStore{}, &fakePreparer{})
	if _, err := service.Add("tcp", 30001, "host:53", config.ProtocolTCP, config.DirectionOutbound); err != nil { t.Fatal(err) }
	if _, err := service.Add("udp", 30001, "host:53", config.ProtocolUDP, config.DirectionOutbound); err == nil { t.Fatal("duplicate local port accepted") }
}
