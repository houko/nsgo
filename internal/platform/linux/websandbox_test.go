//go:build linux && !android

package linux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebKitSandboxWithAppArmorRestriction(t *testing.T) {
	if webKitPackagedSandbox() {
		t.Skip("WebKit uses a package or container sandbox")
	}
	dir := t.TempDir()
	restriction := filepath.Join(dir, "restriction")
	bwrap := filepath.Join(dir, "bwrap")
	if err := os.WriteFile(bwrap, []byte("#!/bin/sh\nprintf 'namespace denied\\n' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS", "0")
	for _, tc := range []struct {
		name     string
		setting  string
		helper   string
		wantFail bool
	}{
		{name: "no restriction file", helper: bwrap},
		{name: "restriction off", setting: "0\n", helper: bwrap},
		{name: "restriction on and allowed", setting: "1\n", helper: "/usr/bin/true"},
		{name: "restriction on and denied", setting: "1\n", helper: bwrap, wantFail: true},
		{name: "no system helper", setting: "1\n", helper: filepath.Join(dir, "missing")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setting != "" {
				if err := os.WriteFile(restriction, []byte(tc.setting), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			err := checkWebKitSandbox(restriction, tc.helper)
			if tc.wantFail {
				if err == nil || !strings.Contains(err.Error(), "AppArmor's unprivileged user namespace restriction") ||
					!strings.Contains(err.Error(), "namespace denied") || !strings.Contains(err.Error(), "grants userns") {
					t.Fatalf("checkWebKitSandbox = %v, want AppArmor diagnosis and helper output", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
