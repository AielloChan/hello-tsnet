package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	minLocalPort = 1024
	maxPort      = 65535
	maxNameBytes = 80
)

type Protocol string

type Direction string

const (
	DirectionOutbound Direction = "outbound"
	DirectionInbound Direction = "inbound"
)

const (
	ProtocolTCP Protocol = "tcp"
	ProtocolUDP Protocol = "udp"
	ProtocolBoth Protocol = "both"
)

type Rule struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	LocalPort int    `json:"localPort"`
	Target    string `json:"target"`
	Protocol  Protocol `json:"protocol"`
	Direction Direction `json:"direction"`
}

func (r Rule) EffectiveDirection() Direction {
	if r.Direction == "" {
		return DirectionOutbound
	}
	return r.Direction
}

func (r Rule) TargetAddress() string {
	if r.EffectiveDirection() == DirectionInbound {
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(r.LocalPort))
	}
	return r.Target
}

func (r Rule) EffectiveProtocol() Protocol {
	if r.Protocol == "" {
		return ProtocolTCP
	}
	return r.Protocol
}

func (r Rule) Networks() []string {
	switch r.EffectiveProtocol() {
	case ProtocolTCP:
		return []string{"tcp"}
	case ProtocolUDP:
		return []string{"udp"}
	case ProtocolBoth:
		return []string{"tcp", "udp"}
	default:
		return nil
	}
}

func NewRule(name string, localPort int, target string, protocol Protocol, direction Direction) (Rule, error) {
	if direction == "" {
		direction = DirectionOutbound
	}
	if protocol == "" {
		protocol = ProtocolBoth
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return Rule{}, fmt.Errorf("generate rule id: %w", err)
	}
	rule := Rule{
		ID:        hex.EncodeToString(idBytes),
		Name:      strings.TrimSpace(name),
		LocalPort: localPort,
		Target:    strings.TrimSpace(target),
		Protocol:  protocol,
		Direction: direction,
	}
	if err := ValidateRule(rule); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

func ValidateRule(rule Rule) error {
	switch rule.Direction {
	case "", DirectionOutbound, DirectionInbound:
	default:
		return fmt.Errorf("direction must be outbound or inbound")
	}
	switch rule.Protocol {
	case "", ProtocolTCP, ProtocolUDP, ProtocolBoth:
	default:
		return fmt.Errorf("protocol must be tcp, udp, or both")
	}
	if strings.TrimSpace(rule.ID) == "" {
		return fmt.Errorf("rule id is required")
	}
	name := strings.TrimSpace(rule.Name)
	if name == "" {
		return fmt.Errorf("rule name is required")
	}
	if len(name) > maxNameBytes {
		return fmt.Errorf("rule name must be at most %d bytes", maxNameBytes)
	}
	minimum := minLocalPort
	if rule.EffectiveDirection() == DirectionInbound {
		minimum = 1
	}
	if rule.LocalPort < minimum || rule.LocalPort > maxPort {
		return fmt.Errorf("local port must be between %d and %d", minimum, maxPort)
	}
	target := strings.TrimSpace(rule.Target)
	if rule.EffectiveDirection() == DirectionInbound {
		if target != "" {
			return fmt.Errorf("inbound rules must not specify a target")
		}
		return nil
	}
	if strings.Contains(target, "://") {
		return fmt.Errorf("target host must not include a URL scheme")
	}
	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		return fmt.Errorf("target must be host:port: %w", err)
	}
	if host == "" || strings.ContainsAny(host, " \t\r\n/") {
		return fmt.Errorf("target host is invalid")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > maxPort {
		return fmt.Errorf("target port must be between 1 and %d", maxPort)
	}
	return nil
}

func ValidateRules(rules []Rule) error {
	ids := make(map[string]struct{}, len(rules))
	ports := make(map[int]struct{}, len(rules))
	for i, rule := range rules {
		if err := ValidateRule(rule); err != nil {
			return fmt.Errorf("rule %d: %w", i+1, err)
		}
		if _, exists := ids[rule.ID]; exists {
			return fmt.Errorf("duplicate rule id %q", rule.ID)
		}
		ids[rule.ID] = struct{}{}
		if _, exists := ports[rule.LocalPort]; exists {
			return fmt.Errorf("duplicate local port %d", rule.LocalPort)
		}
		ports[rule.LocalPort] = struct{}{}
	}
	return nil
}
