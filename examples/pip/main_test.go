package main

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestScriptContentAndPiPControls(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("headless support model")
	}
	app := nsgotest.Start(t, Root, nsgotest.Environment(core.Environment{OS: core.OSIOS}), nsgotest.PictureInPictureSupport(true, true))
	app.Press("id:start-video")
	if !app.Find("id:video").PiPState.Active {
		t.Fatal("video did not enter PiP")
	}
	app.Press("text:Next line")
	app.FindMounted("text:Line 2")
	app.Press("id:start-script")
	if app.Find("id:script-status").Text != "Picture in Picture is active" {
		t.Fatal("script did not enter PiP")
	}
}
