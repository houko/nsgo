//go:build windows

package windows

import "testing"

func TestWindowsCommandArg(t *testing.T) {
	for _, test := range []struct {
		arg, want string
	}{
		{`C:\\Program Files\\NSGo\\app.exe`, `"C:\\Program Files\\NSGo\\app.exe"`},
		{`C:\\NSGo\\app.exe`, `C:\\NSGo\\app.exe`},
		{`C:\\Program Files\\A\\`, `"C:\\Program Files\\A\\\\"`},
	} {
		if got := windowsCommandArg(test.arg); got != test.want {
			t.Errorf("windowsCommandArg(%q) = %q, want %q", test.arg, got, test.want)
		}
	}
}

func TestLoginValueName(t *testing.T) {
	if got := loginValueName("dev.nsgo.app", `C:\\app.exe`); got != "dev.nsgo.app" {
		t.Fatalf("ID value name %q", got)
	}
	if got := loginValueName("", `C:\\Program Files\\app.exe`); got != "app" {
		t.Fatalf("fallback value name %q", got)
	}
}
