//go:build linux && !android

package linux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoginEntryRoundTrip(t *testing.T) {
	entry := loginEntry{path: filepath.Join(t.TempDir(), "autostart", "app.desktop"), name: "A demo", executable: "/Applications/A demo\\tool"}
	if entry.enabled() {
		t.Fatal("missing entry is enabled")
	}
	if err := entry.set(true); err != nil {
		t.Fatal(err)
	}
	if !entry.enabled() {
		t.Fatal("written entry is not enabled")
	}
	data, err := os.ReadFile(entry.path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `Exec="/Applications/A demo\\tool" --nsgo-launched-at-login`) {
		t.Fatalf("entry Exec = %q", text)
	}
	if err := entry.set(false); err != nil {
		t.Fatal(err)
	}
	if entry.enabled() {
		t.Fatal("removed entry is enabled")
	}
	if err := entry.set(false); err != nil {
		t.Fatal("removing a missing entry: ", err)
	}
}

func TestAppLoginEntryUsesXDGConfig(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	e, ok := appLoginEntry("dev.nsgo.test", "Test app", "/tmp/app")
	if !ok || e.path != filepath.Join(config, "autostart", "dev.nsgo.test.desktop") {
		t.Fatalf("entry = %+v, ok %v", e, ok)
	}
	if _, ok := appLoginEntry("bad/name", "Test app", "/tmp/app"); ok {
		t.Fatal("path separator accepted in autostart ID")
	}
}
