package routehelper

import (
	"errors"
	"net/netip"
	"reflect"
	"testing"
)

func TestRouteUpdatesTouchOnlyChangedAddressesAndRetryFailures(t *testing.T) {
	a := netip.MustParseAddr("100.64.0.1")
	b := netip.MustParseAddr("fd7a:115c:a1e0::2")
	c := netip.MustParseAddr("100.64.0.3")
	var changes []string
	fail := true
	update := func(action string, addr netip.Addr) error {
		changes = append(changes, action+" "+addr.String())
		if fail && addr == c {
			return errors.New("conflicting route")
		}
		return nil
	}
	r := routeSet{installed: map[netip.Addr]bool{}}
	if err := r.update([]netip.Addr{a, b}, update); err != nil {
		t.Fatal(err)
	}
	if err := r.update([]netip.Addr{b, c}, update); err == nil {
		t.Fatal("did not report conflict")
	}
	fail = false
	if err := r.update([]netip.Addr{b, c}, update); err != nil {
		t.Fatal(err)
	}
	want := []string{"add " + a.String(), "add " + b.String(), "delete " + a.String(), "add " + c.String(), "add " + c.String()}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("route operations = %v, want %v", changes, want)
	}
}
