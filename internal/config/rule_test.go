package config

import (
	"strings"
	"testing"
)

func TestNewRuleAcceptsTailnetTarget(t *testing.T) {
	rule, err := NewRule("Screen Sharing", 30001, "screen-share.example.net:5900", ProtocolTCP, DirectionOutbound)
	if err != nil {
		t.Fatalf("NewRule() error = %v", err)
	}
	if rule.ID == "" {
		t.Fatal("NewRule() returned an empty ID")
	}
	if rule.Name != "Screen Sharing" || rule.LocalPort != 30001 || rule.Target != "screen-share.example.net:5900" {
		t.Fatalf("NewRule() = %#v", rule)
	}
}

func TestValidateRuleRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name string
		rule Rule
		want string
	}{
		{name: "missing id", rule: Rule{Name: "ssh", LocalPort: 30001, Target: "host:22"}, want: "id"},
		{name: "blank name", rule: Rule{ID: "a", Name: "  ", LocalPort: 30001, Target: "host:22"}, want: "name"},
		{name: "privileged port", rule: Rule{ID: "a", Name: "ssh", LocalPort: 22, Target: "host:22"}, want: "local port"},
		{name: "large port", rule: Rule{ID: "a", Name: "ssh", LocalPort: 70000, Target: "host:22"}, want: "local port"},
		{name: "missing target port", rule: Rule{ID: "a", Name: "ssh", LocalPort: 30001, Target: "host"}, want: "target"},
		{name: "bad target port", rule: Rule{ID: "a", Name: "ssh", LocalPort: 30001, Target: "host:nope"}, want: "target port"},
		{name: "target scheme", rule: Rule{ID: "a", Name: "ssh", LocalPort: 30001, Target: "https://host:443"}, want: "target host"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRule(tt.rule)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tt.want) {
				t.Fatalf("ValidateRule() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateRulesRejectsDuplicatePortsAndIDs(t *testing.T) {
	base := Rule{ID: "one", Name: "one", LocalPort: 30001, Target: "host:22"}

	tests := []struct {
		name  string
		rules []Rule
		want  string
	}{
		{
			name:  "duplicate ports",
			rules: []Rule{base, {ID: "two", Name: "two", LocalPort: 30001, Target: "host:80"}},
			want:  "local port",
		},
		{
			name:  "duplicate ids",
			rules: []Rule{base, {ID: "one", Name: "two", LocalPort: 30002, Target: "host:80"}},
			want:  "id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRules(tt.rules)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tt.want) {
				t.Fatalf("ValidateRules() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
