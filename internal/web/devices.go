package web

import (
	"context"
	"net/http"
	"time"

	"hello-tsnet/internal/tailnet"
)

type DeviceReader func(context.Context) ([]tailnet.Device, error)

func devicesHandler(read DeviceReader) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if read == nil {
			writeError(w, http.StatusServiceUnavailable, tailnet.ErrDevicesUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
		defer cancel()
		devices, err := read(ctx)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, tailnet.ErrDevicesUnavailable)
			return
		}
		if devices == nil {
			devices = []tailnet.Device{}
		}
		writeJSON(w, http.StatusOK, devices)
	}
}
