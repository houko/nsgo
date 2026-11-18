//go:build windows

package windows

import (
	"fmt"
	"os"
	"syscall"
	"testing"
	"unsafe"
)

func TestWindowsCommandArg(t *testing.T) {
	for _, test := range []struct {
		arg, want string
	}{
		{`C:\Program Files\NSGo\app.exe`, `"C:\Program Files\NSGo\app.exe"`},
		{`C:\NSGo\app.exe`, `C:\NSGo\app.exe`},
		{`C:\Program Files\A\`, `"C:\Program Files\A\\"`},
		{`C:\A "B"\app.exe`, `"C:\A \"B\"\app.exe"`},
	} {
		if got := windowsCommandArg(test.arg); got != test.want {
			t.Errorf("windowsCommandArg(%q) = %q, want %q", test.arg, got, test.want)
		}
	}
}

func TestLoginValueName(t *testing.T) {
	if got := loginValueName("dev.nsgo.app", `C:\app.exe`); got != "dev.nsgo.app" {
		t.Fatalf("ID value name %q", got)
	}
	if got := loginValueName("", `C:\Program Files\my.app.exe`); got != "my.app" {
		t.Fatalf("fallback value name %q", got)
	}
}

func TestLoginTurnedOff(t *testing.T) {
	for _, test := range []struct {
		approved []byte
		off      bool
	}{
		{nil, false},
		{[]byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, false},
		{[]byte{6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, false},
		{[]byte{3, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}, true},
		{[]byte{7, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}, true},
	} {
		if got := loginTurnedOff(test.approved); got != test.off {
			t.Errorf("loginTurnedOff(% x) = %v, want %v", test.approved, got, test.off)
		}
	}
}

// TestLoginEntry writes, reads and deletes a Run value of its own under the user's key, and turns it off as Task Manager's Startup apps does.
func TestLoginEntry(t *testing.T) {
	if packaged() {
		t.Skip("runs in a package")
	}
	id := fmt.Sprintf("dev.nsgo.test-login-%d", os.Getpid())
	t.Cleanup(func() {
		deleteRegValue(loginRunKey, id)
		deleteRegValue(loginApprovedKey, id)
	})
	if launchesAtLogin(id) {
		t.Fatal("a missing entry launches at login")
	}
	if err := setLoginEntry(true, id); err != nil {
		t.Fatal(err)
	}
	if !launchesAtLogin(id) {
		t.Fatal("the written entry does not launch at login")
	}
	exe, _ := os.Executable()
	if data, kind, ok := regValue(loginRunKey, id); !ok || kind != syscall.REG_SZ ||
		syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&data[0])), len(data)/2)) != windowsCommandArg(exe)+" --nsgo-launched-at-login" {
		t.Fatalf("Run value = %q, %d, %v", data, kind, ok)
	}

	// Task Manager turns the app off beside the Run value, which it leaves.
	var key syscall.Handle
	if r, _, _ := regCreateKeyExW.Call(uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(wide(loginApprovedKey))), 0, 0, 0, syscall.KEY_SET_VALUE, 0, uintptr(unsafe.Pointer(&key)), 0); r != 0 {
		t.Fatal(syscall.Errno(r))
	}
	off := []byte{3, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}
	r, _, _ := regSetValueExW.Call(uintptr(key), uintptr(unsafe.Pointer(wide(id))), 0, syscall.REG_BINARY, uintptr(unsafe.Pointer(&off[0])), uintptr(len(off)))
	syscall.RegCloseKey(key)
	if r != 0 {
		t.Fatal(syscall.Errno(r))
	}
	if launchesAtLogin(id) {
		t.Fatal("an entry turned off in Startup apps launches at login")
	}
	if err := setLoginEntry(true, id); err != nil {
		t.Fatal(err)
	}
	if !launchesAtLogin(id) {
		t.Fatal("turning the entry on again left it off")
	}

	if err := setLoginEntry(false, id); err != nil {
		t.Fatal(err)
	}
	if launchesAtLogin(id) {
		t.Fatal("the removed entry launches at login")
	}
	if _, _, ok := regValue(loginRunKey, id); ok {
		t.Fatal("the Run value is left")
	}
	if err := setLoginEntry(false, id); err != nil {
		t.Fatal("removing a missing entry: ", err)
	}
}
