//go:build darwin && cgo

package menubar

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fblocks
#cgo LDFLAGS: -framework AppKit -framework ServiceManagement -framework ImageIO -framework CoreGraphics
#include <stdlib.h>
#include "menu.h"
*/
import "C"

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"
	"unsafe"

	"hello-tsnet/internal/routehelper"
	"hello-tsnet/internal/tailnet"
)

type Menu struct {
	Status  func() tailnet.Status
	Devices func(context.Context) ([]tailnet.Device, error)
	Ping    func(context.Context, string) (time.Duration, error)
	Login   func(context.Context) (string, error)
	Logout  func(context.Context) error

	refreshing  atomic.Bool
	accountBusy atomic.Bool
	latency     latencyMonitor
}

// These callbacks are installed and invoked only on the main AppKit thread.
var activeMenu *Menu
var quitCallback func()

func (m *Menu) Run(quit func()) {
	m.latency.devices = m.Devices
	m.latency.ping = m.Ping
	m.latency.publish = func(session uint64, result latencyResult) {
		data, err := json.Marshal(result)
		if err != nil {
			return
		}
		value := C.CString(string(data))
		defer C.free(unsafe.Pointer(value))
		C.HTMenuLatencyUpdate(value, C.ulonglong(session))
	}
	defer m.latency.stop()
	activeMenu = m
	quitCallback = quit
	defer func() { activeMenu, quitCallback = nil, nil }()
	C.HTMenuRun()
}

func (m *Menu) Finish(err error) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	value := C.CString(message)
	defer C.free(unsafe.Pointer(value))
	C.HTMenuFinish(value)
}

func ShowError(err error) {
	value := C.CString(err.Error())
	defer C.free(unsafe.Pointer(value))
	C.HTShowError(value)
}

//export goMenuQuit
func goMenuQuit() {
	if activeMenu != nil {
		activeMenu.latency.stop()
	}
	if quitCallback != nil {
		quitCallback()
	}
}

//export goMenuSetProbing
func goMenuSetProbing(active C.int) C.ulonglong {
	if activeMenu == nil {
		return 0
	}
	if active == 0 {
		activeMenu.latency.stop()
		return 0
	}
	return C.ulonglong(activeMenu.latency.start())
}

//export goMenuSignedIn
func goMenuSignedIn() C.int {
	if activeMenu != nil && activeMenu.Status().State == tailnet.StateReady {
		return 1
	}
	return 0
}

//export goMenuIconState
func goMenuIconState() C.int {
	if activeMenu == nil {
		return C.HTMenuIconIdle
	}
	status := activeMenu.Status()
	switch status.State {
	case tailnet.StateReady:
		if routehelper.NeedsInstall() {
			return C.HTMenuIconIdle
		}
		if status.RoutesReady {
			return C.HTMenuIconReady
		}
		return C.HTMenuIconLoading
	case tailnet.StatePending:
		if status.AuthURL == "" {
			return C.HTMenuIconLoading
		}
	}
	return C.HTMenuIconIdle
}

//export goMenuAuthorize
func goMenuAuthorize() {
	runAccountAction(false)
}

//export goMenuLogout
func goMenuLogout() {
	runAccountAction(true)
}

func runAccountAction(logout bool) {
	menu := activeMenu
	if menu == nil || !menu.accountBusy.CompareAndSwap(false, true) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var link, message string
		var err error
		var logoutResult C.int
		if logout {
			err = menu.Logout(ctx)
			if err == nil {
				logoutResult = 1
			} else {
				logoutResult = -1
				message = "暂时无法确认退出登录是否完成。"
			}
		} else {
			status := menu.Status()
			if status.State == tailnet.StateError {
				message = status.Error
			} else {
				link, err = menu.Login(ctx)
				if err != nil {
					message = "暂时无法获取授权链接，请稍后重试。"
				}
			}
		}
		url := C.CString(link)
		failure := C.CString(message)
		defer C.free(unsafe.Pointer(url))
		defer C.free(unsafe.Pointer(failure))
		menu.accountBusy.Store(false)
		C.HTMenuAccountFinished(url, failure, logoutResult)
	}()
}

//export goMenuRouteHelperNeeded
func goMenuRouteHelperNeeded() C.int {
	if routehelper.NeedsInstall() {
		return 1
	}
	return 0
}

//export goMenuRouteHelperNeedsUpgrade
func goMenuRouteHelperNeedsUpgrade() C.int {
	if routehelper.NeedsUpgrade() {
		return 1
	}
	return 0
}

//export goMenuInstallRouteHelper
func goMenuInstallRouteHelper() {
	setRouteHelperInstalled(true)
}

//export goMenuUninstallRouteHelper
func goMenuUninstallRouteHelper() {
	setRouteHelperInstalled(false)
}

func setRouteHelperInstalled(install bool) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		err := routehelper.SetInstalled(ctx, install)
		var removing C.int
		if !install {
			removing = 1
		}
		message := ""
		if err != nil {
			message = err.Error()
		}
		value := C.CString(message)
		defer C.free(unsafe.Pointer(value))
		C.HTMenuRouteHelperFinished(value, removing)
	}()
}

//export goMenuRefresh
func goMenuRefresh() {
	menu := activeMenu
	if menu == nil || !menu.refreshing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer menu.refreshing.Store(false)
		snapshot := struct {
			Status      tailnet.Status    `json:"status"`
			Devices     *[]tailnet.Device `json:"devices,omitempty"`
			DeviceError string            `json:"deviceError,omitempty"`
		}{Status: menu.Status()}
		snapshot.Status.AuthURL = ""
		if snapshot.Status.State == tailnet.StateReady {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			devices, err := menu.Devices(ctx)
			cancel()
			if err != nil {
				snapshot.DeviceError = "暂时无法读取设备"
			} else {
				if devices == nil {
					devices = []tailnet.Device{}
				}
				snapshot.Devices = &devices
			}
		}
		data, err := json.Marshal(snapshot)
		if err != nil {
			return
		}
		value := C.CString(string(data))
		defer C.free(unsafe.Pointer(value))
		C.HTMenuUpdate(value)
	}()
}
