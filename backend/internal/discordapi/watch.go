package discordapi

import (
	"html/template"
	"net/http"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/applink"
	"bjoernblessin.de/screenshare/internal/group"
)

// The page a button on a stated activity lands a browser on (docs/discord-mode.md).
//
// Discord opens an https address in a browser, and the app is reached at a scheme of its own,
// so this route stands between the two and carries the browser on (internal/applink).
// It takes no credential: the group id is the public digest every path on the relay carries,
// and what a link names is refused at an app holding no seat in that group (docs/membership.md).

// watchLink is what the page is rendered from.
// template.URL because a scheme outside http is filtered out of an href otherwise,
// and the link is built here rather than taken from the request.
type watchLink struct {
	Link template.URL
}

// The press is the page's own work: a browser hands a link to an app on a gesture,
// and a navigation the page starts by itself is dropped without one.
var watchPage = template.Must(template.New("watch").Parse(`<!DOCTYPE html>
<html lang="en">
<meta charset="utf-8">
<title>Watch on MirrorMe</title>
<h1>Watch on MirrorMe</h1>
<p><a href="{{.Link}}">Open MirrorMe</a></p>
<p>The stream opens in the MirrorMe window on this machine.
Watching needs MirrorMe installed, and a seat in the voice channel the stream is shared in.</p>
<script>location.replace({{.Link}})</script>
`))

// serveWatch answers the page naming one stream of one group.
//
// The name is held against the rule a publish is checked against before the link is built:
// what arrives here is an address somebody typed, where applink asserts over one its caller computed.
func (s *Service) serveWatch(w http.ResponseWriter, r *http.Request) {
	groupID, stream := r.PathValue("group"), r.PathValue("stream")
	if groupID == "" || !group.NameHolds(stream) {
		http.Error(w, "This address names no stream to open.", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := watchPage.Execute(w, watchLink{Link: template.URL(applink.FormatWatch(groupID, stream))}); err != nil {
		logger.Warnf("a watch page did not reach the browser that asked for it: %v", err)
	}
}
