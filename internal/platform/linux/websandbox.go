//go:build linux && !android

package linux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const webKitUserNSRestriction = "/proc/sys/kernel/apparmor_restrict_unprivileged_userns"

// checkWebKitSandbox checks the system bubblewrap where AppArmor restricts
// user namespaces. A WebKit child can trap the app when sandbox setup fails.
func checkWebKitSandbox(restrictionFile, bwrap string) error {
	restriction, err := os.ReadFile(restrictionFile)
	if err != nil || strings.TrimSpace(string(restriction)) != "1" {
		return nil
	}
	// These launches can use a different path in WebKit's process launcher.
	if os.Getenv("WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS") == "1" || webKitPackagedSandbox() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bwrap, "--unshare-user", "--unshare-net", "--ro-bind", "/", "/", "/usr/bin/true").CombinedOutput()
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("WebKitGTK could not start %s with AppArmor's unprivileged user namespace restriction enabled. Ask your administrator to install or load the bubblewrap AppArmor profile that grants userns (%s)", bwrap, detail)
}

// webKitPackagedSandbox reports environments WebKit checks before using bubblewrap.
func webKitPackagedSandbox() bool {
	if _, err := os.Stat("/.flatpak-info"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	_, snap := os.LookupEnv("SNAP")
	_, name := os.LookupEnv("SNAP_NAME")
	_, revision := os.LookupEnv("SNAP_REVISION")
	return snap && name && revision
}
