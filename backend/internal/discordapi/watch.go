package discordapi

import (
	"html/template"
	"net/http"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/applink"
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
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Watch on MirrorMe</title>
<style>
body { margin: 0; display: grid; place-items: center; min-height: 100vh;
       font: 16px/1.5 system-ui, sans-serif; background: #17181c; color: #e6e7ea; }
main { max-width: 30rem; padding: 2rem; text-align: center; }
h1 { font-size: 1.5rem; margin: 0 0 0.75rem; }
p { margin: 0 0 1.5rem; color: #b4b7bf; }
p.fine { margin: 1.5rem 0 0; font-size: 0.875rem; }
a.open { display: inline-block; padding: 0.75rem 1.5rem; border-radius: 0.5rem;
         background: #3ba55d; color: #fff; font-weight: 600; text-decoration: none; }
</style>
<main>
<h1>Watch on MirrorMe</h1>
<p>The stream opens in the MirrorMe window on this machine.</p>
<a class="open" href="{{.Link}}">Open MirrorMe</a>
<p class="fine">Watching needs MirrorMe installed, and a seat in the voice channel the stream is shared in.</p>
</main>
<script>location.replace({{.Link}})</script>
`))

// serveWatch answers the page naming one stream of one group.
func (s *Service) serveWatch(w http.ResponseWriter, r *http.Request) {
	group, stream := r.PathValue("group"), r.PathValue("stream")
	if group == "" || stream == "" {
		http.Error(w, "This address names no stream to open.", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := watchPage.Execute(w, watchLink{Link: template.URL(applink.FormatWatch(group, stream))}); err != nil {
		logger.Warnf("a watch page did not reach the browser that asked for it: %v", err)
	}
}
