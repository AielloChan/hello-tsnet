package config

import (
	"os"
	"path/filepath"
	"strings"
	"strconv"
	"testing"
)

func TestInboundRulesOnlyTargetSameLoopbackPort(t *testing.T) {
	for _, port := range []int{1, 53, 8080, 65535} {
		rule, err := NewRule("local", port, "", ProtocolBoth, DirectionInbound)
		if err != nil { t.Fatal(err) }
		if rule.Direction != DirectionInbound || rule.TargetAddress() != "127.0.0.1:"+strconv.Itoa(port) { t.Fatalf("inbound = %#v, address %q", rule, rule.TargetAddress()) }
	}
	for _, target := range []string{"host:80", "127.0.0.1:80", "[::1]:80"} {
		if _, err := NewRule("local", 8080, target, ProtocolTCP, DirectionInbound); err == nil { t.Fatalf("inbound accepted target %q", target) }
	}
	if _, err := NewRule("local", 0, "", ProtocolTCP, DirectionInbound); err == nil { t.Fatal("accepted zero port") }
	if _, err := NewRule("local", 65536, "", ProtocolTCP, DirectionInbound); err == nil { t.Fatal("accepted oversized port") }
	if _, err := NewRule("local", 8080, "host:80", ProtocolTCP, "invalid"); err == nil { t.Fatal("accepted invalid direction") }
	legacy := Rule{ID: "legacy", Name: "old", LocalPort: 30001, Target: "host:80"}
	if legacy.EffectiveDirection() != DirectionOutbound || legacy.TargetAddress() != legacy.Target { t.Fatalf("legacy = %#v", legacy) }
	created, err := NewRule("out", 30001, "host:80", "", "")
	if err != nil || created.Direction != DirectionOutbound { t.Fatalf("default = %#v, %v", created, err) }
	if err := ValidateRules([]Rule{legacy, {ID: "in", Name: "in", LocalPort: 30001, Direction: DirectionInbound}}); err == nil { t.Fatal("accepted port shared across directions") }
}

func TestStoreMigratesOldDirectionsWithoutWriting(t *testing.T) {
	for _, body := range []string{
		`{"version":1,"rules":[{"id":"one","name":"ssh","localPort":30001,"target":"host:22"}]}`,
		`{"version":2,"rules":[{"id":"one","name":"ssh","localPort":30001,"target":"host:22","protocol":"udp"}]}`,
	} {
		path := filepath.Join(t.TempDir(), "rules.json")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil { t.Fatal(err) }
		items, err := NewStore(path).Load()
		if err != nil || len(items) != 1 || items[0].Direction != DirectionOutbound { t.Fatalf("Load = %#v, %v", items, err) }
		if strings.Contains(body, `"version":2`) && items[0].Protocol != ProtocolUDP { t.Fatal("v2 protocol changed") }
		after, err := os.ReadFile(path)
		if err != nil || string(after) != body { t.Fatalf("Load wrote old file: %s, %v", after, err) }
	}
}

func TestStoreRejectsAmbiguousDirectionVersions(t *testing.T) {
	for _, body := range []string{
		`{"version":1,"rules":[{"id":"one","name":"ssh","localPort":30001,"target":"host:22","direction":"inbound"}]}`,
		`{"version":2,"rules":[{"id":"one","name":"ssh","localPort":30001,"target":"host:22","protocol":"tcp","direction":"outbound"}]}`,
		`{"version":3,"rules":[{"id":"one","name":"ssh","localPort":30001,"target":"host:22","protocol":"tcp"}]}`,
		`{"version":3,"rules":[{"id":"one","name":"ssh","localPort":30001,"target":"host:22","protocol":"tcp","direction":"invalid"}]}`,
	} {
		path := filepath.Join(t.TempDir(), "rules.json")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil { t.Fatal(err) }
		if _, err := NewStore(path).Load(); err == nil { t.Fatalf("accepted %s", body) }
	}
}

func TestStoreVersionThreeRoundTripsInbound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	store := NewStore(path)
	rule, err := NewRule("dns", 53, "", ProtocolBoth, DirectionInbound)
	if err != nil { t.Fatal(err) }
	if err := store.Save([]Rule{rule}); err != nil { t.Fatal(err) }
	items, err := store.Load()
	if err != nil || len(items) != 1 || items[0] != rule { t.Fatalf("Load = %#v, %v", items, err) }
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `"version": 3`) { t.Fatalf("version = %s, %v", data, err) }
}
