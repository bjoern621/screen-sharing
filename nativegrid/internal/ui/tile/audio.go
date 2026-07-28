package tile

import (
	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
)

// fullVolume is the level a tile opens at, the web tile's unattenuated playback.
const fullVolume = 1.0

// Attach hands the tile the player it draws, and nil the absence of one.
//
// nil is a state the model reaches: a factory failure leaves the entry without a player,
// and a restart stops the one it held. The tile drops the reference with it, because a
// stopped pipeline still answers its setters and every later render pass would write mute
// and volume into it.
//
// The grid attaches on every state its stream reports, so the player the tile already
// has is nothing to do: handing the render size over again renegotiates the branch behind
// the scaler, which a state flap is no reason for.
//
// A player that differs is the first one or a retry's.
// Both render at the size the source sends and start at the pipeline's own audio
// defaults, so the tile puts what it shows onto them.
func (t *Tile) Attach(p player.Player) {
	if t.current == p {
		return
	}
	t.current = p
	// What the last player was told is forgotten, so the size the tile already has
	// goes over at once rather than after the heartbeat settles again.
	t.sentW, t.sentH = 0, 0
	if w, h := t.pictureSize(); t.current != nil && w > 0 && h > 0 {
		t.pushSize(w, h)
	}
	t.apply()
}

// SetAudioAvailable offers or withdraws the volume control.
// It stays hidden until the stream turns out to carry audio, like the web tile hides
// VolumeControl on a video-only sink.
func (t *Tile) SetAudioAvailable(on bool) {
	t.audio = on
	t.apply()
}

// setMuted silences the tile and marks it: the button's face flips and the corner chip
// grows the web tile's volume-off marker.
func (t *Tile) setMuted(muted bool) {
	t.muted = muted
	t.apply()
}

// applyAudio hands the tile's mute and volume to the player behind it, the render pass's
// counterpart for the state that lives in a pipeline rather than in a widget.
//
// Both go over on every pass because neither the player nor its audio branch is there
// from the start: a retry brings a pipeline that defaults to unmuted at full volume, and
// the branch itself appears while the stream plays.
// A player without a branch drops what it is told, so the pass after the branch appears
// is what makes the tile's state true of it.
func (t *Tile) applyAudio() {
	if t.current == nil {
		return
	}
	assert.Assert(t.level >= 0 && t.level <= fullVolume, "a tile plays at a fraction of full volume", t.level)

	t.current.SetMuted(t.muted)
	t.current.SetVolume(t.level)
}
