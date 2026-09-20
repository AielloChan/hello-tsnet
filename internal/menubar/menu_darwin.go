//go:build darwin && cgo

package menubar

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fblocks
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>
#include "menu.h"
*/
import "C"

import "unsafe"

type Menu struct {
	ControlURL string
}

// AppKit invokes this only on the main thread, while Run owns the callback.
var quitCallback func()

func (m *Menu) Run(quit func()) {
	quitCallback = quit
	defer func() { quitCallback = nil }()
	url := C.CString(m.ControlURL)
	defer C.free(unsafe.Pointer(url))
	C.HTMenuRun(url)
}

func (m *Menu) Finish(err error) {
	message := ""
	if err != nil { message = err.Error() }
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
	if quitCallback != nil { quitCallback() }
}
