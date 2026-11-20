package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/harmony"
)

// testProject gives a target test its own installed application identity, so installing a test build never replaces the app the developer is testing. An App Clip's ID moves under it too, as iOS installs a clip only under its app's ID.
func testProject(p *project) *project {
	t := *p
	t.ID = testBundleID(p.ID)
	if p.AppClip != nil {
		clip := *p.AppClip
		clip.ID = t.ID + strings.TrimPrefix(clip.ID, p.ID)
		t.AppClip = &clip
	}
	return &t
}

func testBundleID(id string) string {
	candidate := id + ".nsgotest"
	if _, err := harmony.BundleName(candidate); err == nil {
		return candidate
	}
	sum := sha256.Sum256([]byte(id))
	return "dev.nsgo.test." + hex.EncodeToString(sum[:8])
}
