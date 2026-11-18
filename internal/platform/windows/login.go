//go:build windows

package windows

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const loginRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`

var regDeleteValueW = advapi32.NewProc("RegDeleteValueW")

func registryError(result uintptr) error {
	if result == 0 {
		return nil
	}
	return syscall.Errno(result)
}

// loginValueName gives the Run value a stable name even for an app built
// without an ID.
func loginValueName(id, executable string) string {
	if id != "" {
		return id
	}
	name := filepath.Base(executable)
	if ext := filepath.Ext(name); ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	if name == "" {
		return "NSGo"
	}
	return name
}

// windowsCommandArg quotes one argument using CommandLineToArgvW's rules.
func windowsCommandArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\v\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, r := range s {
		switch r {
		case '\\':
			slashes++
		case '"':
			b.WriteString(strings.Repeat("\\", slashes*2+1))
			b.WriteRune(r)
			slashes = 0
		default:
			b.WriteString(strings.Repeat("\\", slashes))
			b.WriteRune(r)
			slashes = 0
		}
	}
	b.WriteString(strings.Repeat("\\", slashes*2))
	b.WriteByte('"')
	return b.String()
}

func loginCommand(executable string) string {
	return windowsCommandArg(executable) + " --nsgo-launched-at-login"
}

func setLaunchAtLogin(on bool, id string) {
	executable, err := os.Executable()
	if err != nil {
		log.Printf("nsgo: launch at login: %v", err)
		return
	}
	name := loginValueName(id, executable)
	var key syscall.Handle
	if on {
		result, _, _ := regCreateKeyExW.Call(uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(wide(loginRunKey))), 0, 0, 0, syscall.KEY_SET_VALUE, 0, uintptr(unsafe.Pointer(&key)), 0)
		if err := registryError(result); err != nil {
			log.Printf("nsgo: launch at login: %v", err)
			return
		}
		defer syscall.RegCloseKey(key)
		value := utf16.Encode([]rune(loginCommand(executable) + "\x00"))
		result, _, _ = regSetValueExW.Call(uintptr(key), uintptr(unsafe.Pointer(wide(name))), 0, syscall.REG_SZ, uintptr(unsafe.Pointer(&value[0])), uintptr(len(value)*2))
		if err := registryError(result); err != nil {
			log.Printf("nsgo: launch at login: %v", err)
		}
		return
	}
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, wide(loginRunKey), 0, syscall.KEY_SET_VALUE, &key); err != nil {
		if err != syscall.ERROR_FILE_NOT_FOUND {
			log.Printf("nsgo: launch at login: %v", err)
		}
		return
	}
	defer syscall.RegCloseKey(key)
	result, _, _ := regDeleteValueW.Call(uintptr(key), uintptr(unsafe.Pointer(wide(name))))
	if err := registryError(result); err != nil && err != syscall.ERROR_FILE_NOT_FOUND {
		log.Printf("nsgo: launch at login: %v", err)
	}
}

func launchesAtLogin(id string) bool {
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	var key syscall.Handle
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, wide(loginRunKey), 0, syscall.KEY_READ, &key); err != nil {
		return false
	}
	defer syscall.RegCloseKey(key)
	var kind, size uint32
	if err := syscall.RegQueryValueEx(key, wide(loginValueName(id, executable)), nil, &kind, nil, &size); err != nil || kind != syscall.REG_SZ || size == 0 {
		return false
	}
	buf := make([]uint16, size/2)
	if err := syscall.RegQueryValueEx(key, wide(loginValueName(id, executable)), nil, &kind, (*byte)(unsafe.Pointer(&buf[0])), &size); err != nil {
		return false
	}
	return syscall.UTF16ToString(buf) == loginCommand(executable)
}
