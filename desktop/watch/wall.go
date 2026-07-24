package watch

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// WallExe launches the native grid: one gst-launch-1.0 process composites
// every watched stream into a single window. Decoding stays native, so the
// wall plays whatever the publisher encoded, including the H.265 4:4:4 and
// RGB (RExt) streams no browser path decodes.
const WallExe = "gst-launch-1.0"

// wallWidth and wallHeight size the composited canvas the tiles divide. The
// sink window scales the canvas to whatever size the window manager gives it.
const (
	wallWidth  = 1920
	wallHeight = 1080
)

// tile is one stream's rectangle on the wall canvas.
type tile struct {
	x, y, w, h int
}

// wallLayout returns the near-square grid for n streams, row-major, with an
// incomplete last row centered (the same shape as the frontend's WHEP grid).
func wallLayout(n int) []tile {
	cols := int(math.Ceil(math.Sqrt(float64(n))))
	rows := (n + cols - 1) / cols
	w := wallWidth / cols
	h := wallHeight / rows

	tiles := make([]tile, 0, n)
	for i := range n {
		row := i / cols
		col := i % cols
		inRow := cols
		if row == rows-1 {
			inRow = n - row*cols
		}
		xoff := (wallWidth - inRow*w) / 2
		tiles = append(tiles, tile{x: xoff + col*w, y: row * h, w: w, h: h})
	}
	return tiles
}

// gstQuote wraps v in double quotes for gst-launch's own tokenizer, so a value
// with spaces survives argv joining. The process is spawned without a shell;
// only gst_parse_launch sees the quotes.
func gstQuote(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(v) + `"`
}

// BuildWallArgs returns the gst-launch-1.0 arguments that render the named
// streams, received over the named transport, as one composited grid window.
//
// Each branch decodes through decodebin, so codec and chroma follow whatever
// the publisher sent. The leaky queue decouples the branches: a stalled stream
// drops its own frames instead of blocking the compositor. decodebin's audio
// pads stay unlinked and are ignored while the video pad flows; the wall is
// video-only.
//
// sizing-policy=keep-aspect-ratio (GStreamer 1.20+) letterboxes each tile.
// The layout is fixed at launch: gst-launch cannot re-position pads at
// runtime, so a changed stream set relaunches the wall.
func BuildWallArgs(s settings.Stream, streamNames []string, transportName string) ([]string, error) {
	if len(streamNames) == 0 {
		return nil, fmt.Errorf("the wall needs at least one stream")
	}

	tiles := wallLayout(len(streamNames))

	args := []string{"compositor", "name=comp", "background=black"}
	for i, t := range tiles {
		pad := "sink_" + strconv.Itoa(i)
		args = append(args,
			pad+"::xpos="+strconv.Itoa(t.x),
			pad+"::ypos="+strconv.Itoa(t.y),
			pad+"::width="+strconv.Itoa(t.w),
			pad+"::height="+strconv.Itoa(t.h),
			pad+"::sizing-policy=keep-aspect-ratio",
		)
	}
	args = append(args, "!", "videoconvert", "!", "autovideosink")

	for i, name := range streamNames {
		src, ok := transport.GstSource(transportName, s, name)
		if !ok {
			return nil, fmt.Errorf("transport %q has no GStreamer watch form", transportName)
		}
		args = append(args, src...)
		// videoconvert directly after decodebin keeps gst-launch's delayed
		// linking off the audio pads: their caps never match, so only the
		// video pad joins the branch.
		args = append(args,
			"!", "decodebin",
			"!", "videoconvert",
			"!", "queue", "max-size-buffers=3", "leaky=downstream",
			"!", "textoverlay", "text="+gstQuote(name), "valignment=top", "halignment=left",
			"!", "comp.sink_"+strconv.Itoa(i),
		)
	}
	return args, nil
}
