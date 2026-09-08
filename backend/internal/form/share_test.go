package form

import (
	"slices"
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/share"
	"bjoernblessin.de/screenshare/internal/window"
)

// shareDeps is a Windows machine with two outputs and one window open,
// the session every share kind is reachable on.
func shareDeps() Deps {
	d := fieldTestDeps()
	d.Platform = platform.Info{OS: "windows"}
	d.Windows = []window.Window{{Handle: 4242, Title: "Notes", App: "notepad", Width: 800, Height: 600}}
	return d
}

// shareDraft is a draft capturing what kind names, on the Windows GStreamer backend.
func shareDraft(kind string) settings.Settings {
	s := availabilityDraft("d3d11screencapturesrc", "hevc_nvenc", "yuv420p", "srt")
	s.Publish.ShareKind = kind
	return s
}

// The three kinds are one control, so a reader picks what to share before picking which one.
func TestTheShareKindOffersWhatTheBackendReads(t *testing.T) {
	d := shareDeps()
	s := shareDraft(share.Monitor)

	for _, kind := range share.Kinds {
		enabled, reason := optionState(d, s, KeyShareKind, kind, noEntry)
		if !enabled {
			t.Errorf("%s reads %s and the control greys it: %v", s.Publish.Capture, kind, reason)
		}
	}
}

// Desktop Duplication hands out an output at a time and holds no windows,
// so the window kind greys with the backend named.
func TestAKindTheBackendCannotReadGreys(t *testing.T) {
	d := shareDeps()
	s := shareDraft(share.Monitor)
	s.Publish.Capture = "ddagrab"

	enabled, reason := optionState(d, s, KeyShareKind, share.Window, noEntry)
	if enabled {
		t.Fatal("ddagrab reads no window and the control offers one")
	}
	if got := reason.GetCode(); got != screensharev1.TextCode_TEXT_CODE_CAPTURE_TAKES_NO_SHARE_KIND {
		t.Errorf("the refusal is %s", got)
	}
}

// The compositor draws the picker there, so nothing on this side names what is captured
// and the whole control is greyed with that as its reason.
func TestThePortalAnswersWhatIsSharedItself(t *testing.T) {
	d := fieldTestDeps()
	d.Platform = platform.Info{OS: "linux", Display: "wayland"}
	s := availabilityDraft("portal", "libx264", "yuv420p", "rtsp")

	st := fieldState(d, s, KeyShareKind, noEntry)
	if st.enabled {
		t.Fatal("the portal decides what is shared and the control is live")
	}
	if got := st.reason.GetCode(); got != screensharev1.TextCode_TEXT_CODE_CAPTURE_ASKS_WHAT_TO_SHARE {
		t.Errorf("the refusal is %s", got)
	}
}

// Each kind names its own target, so the two it does not name are drawn nowhere:
// a monitor list under a window capture decides nothing a reader could act on.
func TestOnlyTheKindsOwnTargetIsShown(t *testing.T) {
	d := shareDeps()
	for _, tc := range []struct {
		kind  string
		shown string
	}{
		{share.Monitor, KeyMonitor},
		{share.Window, KeyShareWindow},
		{share.Region, KeyShareRegion},
	} {
		s := shareDraft(tc.kind)
		for _, key := range []string{KeyMonitor, KeyShareWindow, KeyShareRegion} {
			st := fieldState(d, s, key, noEntry)
			if want := key == tc.shown; st.visible != want {
				t.Errorf("capturing a %s draws %s: %v, want %v", tc.kind, key, st.visible, want)
			}
		}
	}
}

// The window list is the enumeration, so a machine that lists none says so on the control
// rather than drawing an empty dropdown.
func TestAWindowKindNeedsASessionThatListsWindows(t *testing.T) {
	d := fieldTestDeps()
	d.Platform = platform.Info{OS: "linux", Display: "x11"}
	s := shareDraft(share.Window)
	s.Publish.Capture = "ximagesrc"

	enabled, reason := optionState(d, s, KeyShareKind, share.Window, noEntry)
	if enabled {
		t.Fatal("an X11 session lists no windows and the control offers the kind")
	}
	if got := reason.GetCode(); got != screensharev1.TextCode_TEXT_CODE_NO_WINDOW_ENUMERATION {
		t.Errorf("the refusal is %s", got)
	}
}

// The enumeration, and the stored handle beside it while it is still open.
func TestTheWindowListIsTheEnumeration(t *testing.T) {
	d := shareDeps()
	s := shareDraft(share.Window)

	var values []string
	for _, o := range fieldRowFor(t, KeyShareWindow).options(d, s) {
		values = append(values, o.GetValue())
	}
	if want := []string{"4242"}; !slices.Equal(values, want) {
		t.Errorf("the window list is %v, want %v", values, want)
	}
}

// Nothing picked is a stream that cannot start, so it blocks the publish
// rather than being repaired onto a window nobody chose.
func TestAnUnpickedTargetBlocksThePublish(t *testing.T) {
	d := shareDeps()
	for _, kind := range []string{share.Window, share.Region} {
		s := shareDraft(kind)
		if publishable(diagnostics(d, s, nil, nil)) {
			t.Errorf("a %s capture with nothing picked is publishable", kind)
		}
	}
}

// A handle the enumeration does not carry names a closed window,
// which the reader has to see in order to pick another.
func TestAClosedWindowIsNamedRatherThanDropped(t *testing.T) {
	d := shareDeps()
	s := shareDraft(share.Window)
	s.Publish.ShareWindow = "999"

	var closed *screensharev1.FieldOption
	for _, o := range fieldRowFor(t, KeyShareWindow).options(d, s) {
		if o.GetValue() == "999" {
			closed = o
		}
	}
	if closed == nil {
		t.Fatal("the picked window left the list when it closed")
	}
	if got := closed.GetNote().GetCode(); got != screensharev1.TextCode_TEXT_CODE_WINDOW_GONE {
		t.Errorf("the note is %s", got)
	}
}

// Every control of the group carries the same reason where the backend answers for itself:
// nothing on the screen decides what is shared, so a control the reader could move would be a lie.
func TestThePortalLeavesNoShareControlLive(t *testing.T) {
	d := fieldTestDeps()
	d.Platform = platform.Info{OS: "linux", Display: "wayland"}
	s := availabilityDraft("portal", "libx264", "yuv420p", "rtsp")

	for _, key := range []string{KeyShareKind, KeyMonitor, KeyShareWindow, KeyShareRegion} {
		st := fieldState(d, s, key, noEntry)
		if st.visible && st.enabled {
			t.Errorf("%s is the reader's on a capture that answers for itself", key)
		}
	}
}
