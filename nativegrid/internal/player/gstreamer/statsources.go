package gstreamer

import (
	"fmt"
	"strconv"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
)

// statField is one field of an element's "stats" property: the GstStructure key,
// the label the overlay prints for it, and a unit appended to the value.
type statField struct {
	key, label, unit string
}

// statSource names an element factory whose "stats" structure is worth showing
// and the fields to take from it.
type statSource struct {
	factory string
	fields  []statField
}

// statSources is the transport-side half of the overlay. Which of these elements
// a pipeline holds is the transport's business: srtsrc counts an SRT link, the
// jitterbuffers inside rtspsrc count RTP. A field an element does not report is
// skipped, so a key this table has wrong costs its own row and nothing else.
var statSources = []statSource{
	{factory: "srtsrc", fields: []statField{
		{key: "packets-received", label: "packets"},
		{key: "packets-received-lost", label: "lost"},
		{key: "packets-received-retransmitted", label: "retransmitted"},
		{key: "packets-received-dropped", label: "dropped"},
		{key: "receive-rate-mbps", label: "rate", unit: "Mbps"},
		{key: "bandwidth-mbps", label: "link", unit: "Mbps"},
		{key: "rtt-ms", label: "rtt", unit: "ms"},
		{key: "negotiated-latency-ms", label: "buffer", unit: "ms"},
	}},
	{factory: "rtpjitterbuffer", fields: []statField{
		{key: "num-pushed", label: "pushed"},
		{key: "num-lost", label: "lost"},
		{key: "num-late", label: "late"},
		{key: "num-duplicates", label: "duplicates"},
		{key: "rtx-count", label: "rtx sent"},
		{key: "rtx-success-count", label: "rtx recovered"},
	}},
}

// statGroup reads one element's "stats" structure into labelled rows. It reports
// false while the element keeps no counters the table names, so the overlay only
// grows a block once there is something in it.
func statGroup(src elementStats) (player.StatGroup, bool) {
	st, _ := src.element.ObjectProperty("stats").(*gst.Structure)
	if st == nil {
		return player.StatGroup{}, false
	}
	g := player.StatGroup{Name: src.element.GetName()}
	for _, f := range src.fields {
		if !st.HasField(f.key) {
			continue
		}
		g.Rows = append(g.Rows, player.StatRow{
			Label: f.label,
			Value: statValue(st.GetValue(f.key), f.unit),
		})
	}
	return g, len(g.Rows) > 0
}

// statValue formats one stats field. The types are the element's, so counters
// print whole and rates to one decimal.
func statValue(v any, unit string) string {
	var out string
	switch n := v.(type) {
	case float64:
		out = strconv.FormatFloat(n, 'f', 1, 64)
	case float32:
		out = strconv.FormatFloat(float64(n), 'f', 1, 32)
	default:
		out = fmt.Sprint(v)
	}
	if unit != "" {
		out += " " + unit
	}
	return out
}
