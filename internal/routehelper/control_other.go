//go:build !darwin

package routehelper

import (
	"context"
	"errors"
	"net/netip"
	"os"
)

func Open(context.Context, []netip.Addr) (Session, *os.File, error) {
	return nil, nil, errors.New("设备直访需要 macOS")
}
