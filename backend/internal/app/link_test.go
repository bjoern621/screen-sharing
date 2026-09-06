package app

import (
	"errors"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/applink"
	"bjoernblessin.de/screenshare/internal/discordclient"
)

// A link is followed against the group this machine stands in now,
// so these tests land a pass first and then read a link against what it left.

// aStream is the stream the faked manager's index carries.
const aStream = "bob/monitor-0"

// watching is an app in Discord mode, standing in a channel, with a leg to watch over.
func watching(t *testing.T) *App {
	t.Helper()

	a := discordApp(&fakeDiscord{answer: inChannel()})
	a.settings.Viewer.TileWatchTransport = "rtsp"
	a.discordPass()
	return a
}

func TestALinkIntoThisGroupOpensItsStream(t *testing.T) {
	a := watching(t)

	stream, err := a.ResolveLink(applink.FormatWatch(aGroupID, aStream))
	if err != nil {
		t.Fatalf("following a link into this group: %v", err)
	}
	if stream != aStream {
		t.Errorf("the link opens %q, and it names %q", stream, aStream)
	}
}

func TestALinkIntoAnotherChannelNamesTheChannelToJoin(t *testing.T) {
	a := watching(t)

	_, err := a.ResolveLink(applink.FormatWatch("another-group", aStream))
	if err == nil {
		t.Fatal("a link into a group this machine is not in is refused")
	}
	if !strings.Contains(err.Error(), "voice channel") {
		t.Errorf("the refusal names the voice channel to join, said %v", err)
	}
}

func TestALinkOutsideAnyChannelNamesTheChannelToJoin(t *testing.T) {
	a := discordApp(&fakeDiscord{answer: discordclient.Answer{}})
	a.settings.Viewer.TileWatchTransport = "rtsp"
	a.discordPass()

	_, err := a.ResolveLink(applink.FormatWatch(aGroupID, aStream))
	if !errors.Is(err, errNoVoiceChannel) {
		t.Fatalf("a machine in no channel is refused with the channel to join, got %v", err)
	}
}

func TestALinkOnAnUnlinkedMachineNamesTheLink(t *testing.T) {
	a := discordApp(&fakeDiscord{answer: inChannel()})
	a.settings.Relay.DiscordLink = ""
	a.settings.Viewer.TileWatchTransport = "rtsp"
	a.discordPass()

	_, err := a.ResolveLink(applink.FormatWatch(aGroupID, aStream))
	if !errors.Is(err, errDiscordUnlinked) {
		t.Fatalf("an unlinked machine is refused with the link to make, got %v", err)
	}
}

func TestALinkToAStreamNobodyPublishesSaysSo(t *testing.T) {
	a := watching(t)

	_, err := a.ResolveLink(applink.FormatWatch(aGroupID, "bob/gone"))
	if !errors.Is(err, errLinkNotOnRelay) {
		t.Fatalf("a link to a stream the relay does not carry says so, got %v", err)
	}
}

func TestAnUnreadableLinkIsRefused(t *testing.T) {
	a := watching(t)

	if _, err := a.ResolveLink("https://example.test/watch/abc/def"); err == nil {
		t.Error("a web address is not a link this app opens")
	}
}
