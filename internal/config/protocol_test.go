package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRuleProtocolsAndDefault(t *testing.T) {
	for _, tt := range []struct {
		input Protocol
		want Protocol
		networks []string
	}{
		{"", ProtocolBoth, []string{"tcp", "udp"}},
		{ProtocolTCP, ProtocolTCP, []string{"tcp"}},
		{ProtocolUDP, ProtocolUDP, []string{"udp"}},
		{ProtocolBoth, ProtocolBoth, []string{"tcp", "udp"}},
	} {
		rule, err := NewRule("service", 30001, "host:53", tt.input, DirectionOutbound)
		if err != nil || rule.Protocol != tt.want || !reflect.DeepEqual(rule.Networks(), tt.networks) {
			t.Fatalf("NewRule(%q) = %#v, %v", tt.input, rule, err)
		}
	}
	if _, err := NewRule("service", 30001, "host:53", "sctp", DirectionOutbound); err == nil {
		t.Fatal("invalid protocol accepted")
	}
	legacy := Rule{ID: "legacy", Name: "service", LocalPort: 30001, Target: "host:22"}
	if ValidateRule(legacy) != nil || legacy.EffectiveProtocol() != ProtocolTCP || !reflect.DeepEqual(legacy.Networks(), []string{"tcp"}) {
		t.Fatalf("legacy protocol must remain TCP: %#v", legacy)
	}
}

func TestStoreMigratesLegacyTCPWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	original := []byte(`{"version":1,"rules":[{"id":"one","name":"ssh","localPort":30001,"target":"host:22"}]}`)
	if err := os.WriteFile(path, original, 0o600); err != nil { t.Fatal(err) }
	items, err := NewStore(path).Load()
	if err != nil || len(items) != 1 || items[0].Protocol != ProtocolTCP {
		t.Fatalf("Load() = %#v, %v", items, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(original) { t.Fatalf("Load changed file: %q, %v", after, err) }
}

func TestStoreCurrentVersionPreservesProtocolsAndNormalizesLegacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	items := []Rule{
		{ID: "legacy", Name: "legacy", LocalPort: 30001, Target: "host:22"},
		{ID: "udp", Name: "udp", LocalPort: 30002, Target: "host:53", Protocol: ProtocolUDP},
		{ID: "both", Name: "both", LocalPort: 30003, Target: "host:443", Protocol: ProtocolBoth},
	}
	store := NewStore(path)
	if err := store.Save(items); err != nil { t.Fatal(err) }
	if items[0].Protocol != "" { t.Fatal("Save mutated caller's snapshot") }
	contents, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	var data fileData
	if err := json.Unmarshal(contents, &data); err != nil { t.Fatal(err) }
	if data.Version != 3 { t.Fatalf("version = %d, want 3", data.Version) }
	items[0].Protocol = ProtocolTCP
	for i := range items { items[i].Direction = DirectionOutbound }
	loaded, err := store.Load()
	if err != nil || !reflect.DeepEqual(loaded, items) { t.Fatalf("Load() = %#v, %v", loaded, err) }
}

func TestStoreRejectsAmbiguousProtocolVersionsWithoutWriting(t *testing.T) {
	for _, body := range []string{
		`{"version":1,"rules":[{"id":"one","name":"dns","localPort":30001,"target":"host:53","protocol":"udp"}]}`,
		`{"version":2,"rules":[{"id":"one","name":"dns","localPort":30001,"target":"host:53"}]}`,
		`{"version":2,"rules":[{"id":"one","name":"dns","localPort":30001,"target":"host:53","protocol":"sctp"}]}`,
	} {
		path := filepath.Join(t.TempDir(), "rules.json")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil { t.Fatal(err) }
		if _, err := NewStore(path).Load(); err == nil { t.Fatalf("accepted %s", body) }
		after, err := os.ReadFile(path)
		if err != nil || string(after) != body { t.Fatalf("Load changed invalid file: %q, %v", after, err) }
	}
}
