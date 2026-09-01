package portal

import "testing"

// A mask nothing read withholds nothing.
// The publish then reaches SelectSources and the portal's own refusal is the answer,
// which is what a machine nobody asked has to look like.
func TestAnUnreadMaskServesEveryMode(t *testing.T) {
	var none Capabilities

	for _, mode := range []CursorMode{CursorHidden, CursorEmbedded, CursorMetadata} {
		if !none.ServesCursor(mode) {
			t.Errorf("cursor mode %d is withheld by a portal nothing asked", mode)
		}
	}
	if got := none.SourcesServed(SourceMonitor | SourceWindow); got != SourceMonitor|SourceWindow {
		t.Errorf("a portal nothing asked narrowed the source kinds to %d", got)
	}
}

// The compositor's backend fills the masks, so a desktop serving hidden and embedded alone
// is one the metadata mode does not reach.
func TestAMaskWithoutMetadataWithholdsIt(t *testing.T) {
	caps := Capabilities{CursorModes: CursorHidden | CursorEmbedded}

	if !caps.ServesCursor(CursorEmbedded) {
		t.Error("a mode the mask carries is withheld")
	}
	if caps.ServesCursor(CursorMetadata) {
		t.Error("a mode outside the mask is offered, which SelectSources refuses")
	}
}

// A source kind outside the mask fails the whole SelectSources call,
// so what is asked for is narrowed to what the portal answers.
func TestSourceKindsNarrowToWhatThePortalServes(t *testing.T) {
	monitorOnly := Capabilities{SourceTypes: SourceMonitor}

	if got := monitorOnly.SourcesServed(SourceMonitor | SourceWindow); got != SourceMonitor {
		t.Errorf("asked for monitor and window on a monitor-only portal, got %d", got)
	}
	if got := monitorOnly.SourcesServed(SourceWindow); got != 0 {
		t.Errorf("a kind the portal does not serve survived the narrowing as %d", got)
	}
}
