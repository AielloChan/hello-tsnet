package rules

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"hello-tsnet/internal/config"
)

type fakeStore struct {
	mu            sync.Mutex
	saveErr       error
	saved         []config.Rule
	saveCalls     int
	activeSaves   int
	maxSaves      int
	saveDelay     time.Duration
	events        *[]string
}

func (s *fakeStore) Save(items []config.Rule) error {
	s.mu.Lock()
	s.saveCalls++
	s.activeSaves++
	if s.activeSaves > s.maxSaves {
		s.maxSaves = s.activeSaves
	}
	if s.events != nil {
		*s.events = append(*s.events, "save")
	}
	s.mu.Unlock()
	time.Sleep(s.saveDelay)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeSaves--
	if s.saveErr == nil {
		s.saved = append([]config.Rule(nil), items...)
	}
	return s.saveErr
}

type fakePreparer struct {
	err          error
	prepareCalls int
	lastRules    []config.Rule
	change       *fakeChange
	events       *[]string
}

func (p *fakePreparer) Prepare(items []config.Rule) (Change, error) {
	p.prepareCalls++
	p.lastRules = append([]config.Rule(nil), items...)
	if p.events != nil {
		*p.events = append(*p.events, "prepare")
	}
	if p.err != nil {
		return nil, p.err
	}
	change := &fakeChange{events: p.events}
	p.change = change
	return change, nil
}

type fakeChange struct {
	commits int
	aborts  int
	events  *[]string
}

func (c *fakeChange) Commit() {
	c.commits++
	if c.events != nil {
		*c.events = append(*c.events, "commit")
	}
}

func (c *fakeChange) Abort() {
	c.aborts++
	if c.events != nil {
		*c.events = append(*c.events, "abort")
	}
}

func TestAddPrepareFailureDoesNotSave(t *testing.T) {
	store := &fakeStore{}
	preparer := &fakePreparer{err: errors.New("port busy")}
	service := NewService(store, preparer)

	_, err := service.Add("ssh", 30001, "host:22")
	if err == nil {
		t.Fatal("Add() error = nil")
	}
	if store.saveCalls != 0 {
		t.Fatalf("Save() calls = %d, want 0", store.saveCalls)
	}
	if len(service.List()) != 0 {
		t.Fatal("failed Add() changed current rules")
	}
}

func TestAddSaveFailureAbortsPreparedChange(t *testing.T) {
	store := &fakeStore{saveErr: errors.New("disk full")}
	preparer := &fakePreparer{}
	service := NewService(store, preparer)

	_, err := service.Add("ssh", 30001, "host:22")
	if err == nil {
		t.Fatal("Add() error = nil")
	}
	if preparer.change == nil || preparer.change.aborts != 1 || preparer.change.commits != 0 {
		t.Fatalf("change = %#v, want one abort and no commit", preparer.change)
	}
	if len(service.List()) != 0 {
		t.Fatal("failed Add() changed current rules")
	}
}

func TestAddOrdersPrepareSaveCommit(t *testing.T) {
	var events []string
	store := &fakeStore{events: &events}
	preparer := &fakePreparer{events: &events}
	service := NewService(store, preparer)

	added, err := service.Add("ssh", 30001, "host:22")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if added.ID == "" {
		t.Fatal("Add() returned empty ID")
	}
	if !reflect.DeepEqual(events, []string{"prepare", "save", "commit"}) {
		t.Fatalf("events = %v", events)
	}
	if preparer.change.commits != 1 || preparer.change.aborts != 0 {
		t.Fatalf("change = %#v", preparer.change)
	}
}

func TestConcurrentAddsAreSerializedWithoutLostUpdates(t *testing.T) {
	store := &fakeStore{saveDelay: 20 * time.Millisecond}
	preparer := &fakePreparer{}
	service := NewService(store, preparer)

	var group sync.WaitGroup
	errs := make(chan error, 2)
	for _, input := range []struct {
		name string
		port int
	}{{"one", 30001}, {"two", 30002}} {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := service.Add(input.name, input.port, "host:22")
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
	}
	if got := len(service.List()); got != 2 {
		t.Fatalf("List() length = %d, want 2", got)
	}
	if store.maxSaves != 1 {
		t.Fatalf("maximum concurrent Save() calls = %d, want 1", store.maxSaves)
	}
}

func TestRestoreCommitsProvidedRulesWithoutSaving(t *testing.T) {
	want := []config.Rule{{ID: "one", Name: "one", LocalPort: 30001, Target: "host:22"}}
	store := &fakeStore{}
	preparer := &fakePreparer{}
	service := NewService(store, preparer)

	if err := service.Restore(want); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if store.saveCalls != 0 {
		t.Fatalf("Save() calls = %d, want 0", store.saveCalls)
	}
	if preparer.prepareCalls != 1 || preparer.change.commits != 1 {
		t.Fatalf("preparer = %#v", preparer)
	}
	if got := service.List(); !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %#v, want %#v", got, want)
	}
}

func TestRestoreInvalidRulesDoesNotPrepareOrSave(t *testing.T) {
	store := &fakeStore{}
	preparer := &fakePreparer{}
	service := NewService(store, preparer)

	invalid := []config.Rule{{Name: "missing id", LocalPort: 30001, Target: "host:22"}}
	if err := service.Restore(invalid); err == nil {
		t.Fatal("Restore() error = nil")
	}
	if preparer.prepareCalls != 0 || store.saveCalls != 0 {
		t.Fatalf("prepare calls = %d, save calls = %d", preparer.prepareCalls, store.saveCalls)
	}
}

func TestUpdatePreservesIDAndReplacesFields(t *testing.T) {
	store := &fakeStore{}
	preparer := &fakePreparer{}
	service := NewService(store, preparer)
	added, err := service.Add("old", 30001, "host:22")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	updated, err := service.Update(added.ID, "new", 30002, "other:5900")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.ID != added.ID || updated.Name != "new" || updated.LocalPort != 30002 || updated.Target != "other:5900" {
		t.Fatalf("Update() = %#v", updated)
	}
}

func TestDeleteRemovesRuleAndMissingIDReturnsNotFound(t *testing.T) {
	service := NewService(&fakeStore{}, &fakePreparer{})
	added, err := service.Add("one", 30001, "host:22")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := service.Delete(added.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(service.List()) != 0 {
		t.Fatal("Delete() left the rule in List()")
	}
	if err := service.Delete("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}
