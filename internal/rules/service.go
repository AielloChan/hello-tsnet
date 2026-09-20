package rules

import (
	"errors"
	"fmt"
	"sync"

	"hello-tsnet/internal/config"
)

var ErrNotFound = errors.New("rule not found")
var ErrInvalid = errors.New("invalid rule")

type Store interface {
	Save([]config.Rule) error
}

type Change interface {
	Commit()
	Abort()
}

type Preparer interface {
	Prepare([]config.Rule) (Change, error)
}

type Service struct {
	mu       sync.Mutex
	store    Store
	preparer Preparer
	rules    []config.Rule
}

func NewService(store Store, preparer Preparer) *Service {
	return &Service{store: store, preparer: preparer, rules: []config.Rule{}}
}

func (s *Service) Restore(loaded []config.Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := config.ValidateRules(loaded); err != nil {
		return fmt.Errorf("validate restored rules: %w", err)
	}
	change, err := s.preparer.Prepare(loaded)
	if err != nil {
		return fmt.Errorf("prepare restored rules: %w", err)
	}
	change.Commit()
	s.rules = append([]config.Rule(nil), loaded...)
	return nil
}

func (s *Service) List() []config.Rule {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]config.Rule(nil), s.rules...)
}

func (s *Service) Add(name string, localPort int, target string, protocol config.Protocol, direction config.Direction) (config.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rule, err := config.NewRule(name, localPort, target, protocol, direction)
	if err != nil {
		return config.Rule{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	next := append(append([]config.Rule(nil), s.rules...), rule)
	if err := s.replaceLocked(next); err != nil {
		return config.Rule{}, err
	}
	return rule, nil
}

func (s *Service) Update(id, name string, localPort int, target string, protocol config.Protocol, direction config.Direction) (config.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	for i := range s.rules {
		if s.rules[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return config.Rule{}, ErrNotFound
	}
	if protocol == "" {
		protocol = s.rules[index].EffectiveProtocol()
	}
	if direction == "" {
		direction = s.rules[index].EffectiveDirection()
	}
	normalized, err := config.NewRule(name, localPort, target, protocol, direction)
	if err != nil {
		return config.Rule{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	normalized.ID = id
	next := append([]config.Rule(nil), s.rules...)
	next[index] = normalized
	if err := s.replaceLocked(next); err != nil {
		return config.Rule{}, err
	}
	return normalized, nil
}

func (s *Service) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	for i := range s.rules {
		if s.rules[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return ErrNotFound
	}
	next := make([]config.Rule, 0, len(s.rules)-1)
	next = append(next, s.rules[:index]...)
	next = append(next, s.rules[index+1:]...)
	return s.replaceLocked(next)
}

func (s *Service) Replace(items []config.Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.replaceLocked(append([]config.Rule(nil), items...))
}

func (s *Service) replaceLocked(next []config.Rule) error {
	if err := config.ValidateRules(next); err != nil {
		return err
	}
	change, err := s.preparer.Prepare(next)
	if err != nil {
		return fmt.Errorf("prepare rules: %w", err)
	}
	if err := s.store.Save(next); err != nil {
		change.Abort()
		return fmt.Errorf("save rules: %w", err)
	}
	change.Commit()
	s.rules = append([]config.Rule(nil), next...)
	return nil
}
