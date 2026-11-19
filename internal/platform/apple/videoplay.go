//go:build darwin

package apple

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// videoPauseBackground is the backend's own reason for a pause because the app went to the background, which changed reports as VideoPauseInterruption. No audio-session interruption ends it, so it never lets the video resume, and it takes back the resume of an interruption that paused the video before.
const videoPauseBackground = -1

// videoPlayback follows who decides whether a video's player plays. The app's Paused applies when it changes and when a source opens; between changes the platform's controls and the system may pause or play the player, and the player then keeps their state, not Paused, until Paused changes. Each such change is reported once, as EventVideoPause with its reason or EventVideoPlay.
type videoPlayback struct {
	// playing is what the player did last, as far as the backend knows.
	playing bool
	// completed holds a non-repeating end until an explicit replay or seek.
	completed bool
	// end is where that end left the player.
	end time.Duration
	// held says the player's play state is the platform's, not the app's Paused.
	held bool
	// reason is why the system is about to pause the player, given before the player says it stopped; ok when there is one.
	reason    int
	ok        bool
	resumable bool
}

// configure says whether Configure gives the player the app's play state: when Paused changed, or nothing holds it.
func (p *videoPlayback) configure(pausedChanged bool) bool {
	if pausedChanged {
		p.held, p.ok, p.resumable = false, false, false
	}
	return !p.held && (!p.completed || pausedChanged)
}

// applied records the play state the backend is about to give the player, which is not reported.
func (p *videoPlayback) applied(playing bool) {
	p.playing = playing
	if playing {
		p.completed = false
	}
}

// ended records that the player reached its end, which EventEnd reports. A video that repeats plays on from its start, and whoever held its play state keeps it, so a play from the controls while Paused is true goes on; one that does not stops, leaves the play state to Paused again, and stays at its end through unrelated configuration until Paused changes, a seek or the controls play it; at is where it stopped.
func (p *videoPlayback) ended(repeat bool, at time.Duration) {
	if repeat {
		p.playing = true
		return
	}
	p.playing, p.held, p.ok, p.resumable = false, false, false, false
	p.completed, p.end = true, at
}

// rewinds says whether the app's play of a completed video starts over from the beginning, with at the player's position: it does while the player is still where it ended, and resumes where the platform's controls scrubbed it back to otherwise. The end the player reached is the mark, not the item's duration, which a player's end may fall short of.
func (p *videoPlayback) rewinds(at time.Duration) bool {
	return p.completed && at >= p.end
}

// expect records why the system is about to pause the player, before the player says it stopped.
func (p *videoPlayback) expect(reason int) {
	if p.playing {
		p.reason, p.ok = reason, true
	}
}

// changed takes a play state the player reached on its own, from its controls or the system, with paused the app's Paused, and returns the event that reports it. reason is the pause's, unless the system gave one before. A pause for an audio-session interruption may resume when the interruption ends.
func (p *videoPlayback) changed(playing, paused bool, reason int) (driver.Event, bool) {
	background := !playing && reason == videoPauseBackground
	if background {
		// The background's rate notification may come after an interruption already paused the player: it reports nothing, but the video stays paused.
		p.resumable = false
	}
	if playing == p.playing {
		return driver.Event{}, false
	}
	p.playing = playing
	p.held = playing == paused
	if playing {
		p.completed = false
		p.ok, p.resumable = false, false
		return driver.Event{Kind: driver.EventVideoPlay}, true
	}
	if p.ok {
		reason, p.ok = p.reason, false
	}
	// A reason the system gave before stays the one reported, but the background still keeps the pause from resuming.
	p.resumable = p.held && reason == driver.VideoPauseInterruption && !background
	if reason == videoPauseBackground {
		reason = driver.VideoPauseInterruption
	}
	return driver.Event{Kind: driver.EventVideoPause, Index: reason}, true
}

// resume says whether the player plays again as an interruption that paused it ends: when the system lets it, and the app has not changed Paused since.
func (p *videoPlayback) resume() bool {
	ok := p.resumable && p.held && !p.playing
	p.resumable = false
	return ok
}
