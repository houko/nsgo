//go:build linux && !android

package linux

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// loginEntry is the per-user XDG autostart file for an app.
type loginEntry struct {
	path, name, executable string
}

func appLoginEntry(id, name, executable string) (loginEntry, bool) {
	if id == "" {
		id = filepath.Base(executable)
	}
	if id == "" || id == "." || id == string(filepath.Separator) || strings.ContainsAny(id, `/\\`) {
		return loginEntry{}, false
	}
	if name == "" {
		name = id
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return loginEntry{}, false
		}
		config = filepath.Join(home, ".config")
	}
	return loginEntry{path: filepath.Join(config, "autostart", id+".desktop"), name: name, executable: executable}, true
}

func desktopExecArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\"'\\$`") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		if strings.ContainsRune(`\\"'$`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

func (e loginEntry) data() []byte {
	return []byte("[Desktop Entry]\nType=Application\nName=" + desktopValue(e.name) + "\nExec=" + desktopExecArg(e.executable) + " --nsgo-launched-at-login\nX-GNOME-Autostart-enabled=true\n")
}

func desktopValue(s string) string {
	return strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\r", "\\r").Replace(s)
}

func (e loginEntry) set(on bool) error {
	if !on {
		err := os.Remove(e.path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(e.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(e.path), ".nsgo-login-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(e.data()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), e.path)
}

func (e loginEntry) enabled() bool {
	b, err := os.ReadFile(e.path)
	return err == nil && bytes.Equal(b, e.data())
}

func setLaunchAtLogin(on bool, id, name string) {
	executable, err := os.Executable()
	if err != nil {
		log.Printf("nsgo: launch at login: %v", err)
		return
	}
	e, ok := appLoginEntry(id, name, executable)
	if !ok {
		log.Printf("nsgo: launch at login: app ID is not a valid autostart filename")
		return
	}
	if err := e.set(on); err != nil {
		log.Printf("nsgo: launch at login: %v", err)
	}
}

func launchesAtLogin(id, name string) bool {
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	e, ok := appLoginEntry(id, name, executable)
	return ok && e.enabled()
}
