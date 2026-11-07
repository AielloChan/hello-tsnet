package routehelper

import "testing"

func TestConfirmHelperOperation(t *testing.T) {
	tests := []struct {
		name         string
		installed    bool
		needsInstall bool
		needsUpgrade bool
		wantError    bool
	}{
		{name: "installed and ready", installed: true},
		{name: "installed while socket starts", installed: true, needsInstall: true},
		{name: "installation digest mismatch", installed: true, needsUpgrade: true, wantError: true},
		{name: "uninstalled", needsInstall: true},
		{name: "uninstall left socket", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := confirmHelperOperation(tt.installed, func() bool { return tt.needsInstall }, func() bool { return tt.needsUpgrade })
			if (err != nil) != tt.wantError {
				t.Fatalf("got %v, want error: %t", err, tt.wantError)
			}
		})
	}
}
