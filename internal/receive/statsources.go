package receive

import (
	"fmt"
	"strconv"

	"github.com/go-gst/go-gst/pkg/gst"
)

// statField is one field of an element's "stats" property: the GstStructure key,
// the label printed for it, the tooltip that says what the counter measures, and
// a unit appended to the value.
type statField struct {
	key, label, tip, unit string
}

// statSource names an element factory whose "stats" structure is worth showing,
// what the element does, and the fields to take from it.
type statSource struct {
	factory string
	tip     string
	fields  []statField
}

// statSources is the transport-side half of what a receiver reports. Which of
// these elements a pipeline holds is the transport's business: srtsrc counts an
// SRT link, the jitterbuffers inside rtspsrc count RTP. A field an element does
// not report is skipped, so a key this table has wrong costs its own row and
// nothing else.
var statSources = []statSource{
	{
		factory: "srtsrc",
		tip:     "The SRT link this viewer receives on, the watch leg (relay to viewer).",
		fields: []statField{
			{key: "packets-received", label: "packets", tip: "Packets received on the link."},
			{key: "packets-received-lost", label: "lost", tip: "Packets that never arrived, retransmits included."},
			{
				key:   "packets-received-retransmitted",
				label: "retransmitted",
				tip:   "Packets that arrived only after the sender resent them.",
			},
			{
				key:   "packets-received-dropped",
				label: "dropped",
				tip:   "Packets that arrived too late for their play time and were discarded.",
			},
			{key: "receive-rate-mbps", label: "rate", tip: "Measured receive rate on the link.", unit: "Mbps"},
			{key: "bandwidth-mbps", label: "link", tip: "Bandwidth SRT estimates the path can carry.", unit: "Mbps"},
			{
				key:   "rtt-ms",
				label: "rtt",
				tip:   "Round trip time to the relay, which bounds how fast a retransmit can arrive.",
				unit:  "ms",
			},
			{
				key:   "negotiated-latency-ms",
				label: "buffer",
				tip:   "Latency the two sides agreed on: how long SRT holds a packet for retransmits before playing it.",
				unit:  "ms",
			},
		},
	},
	{
		factory: "rtpjitterbuffer",
		tip:     "The buffer that puts RTP packets back in order and asks for the missing ones.",
		fields: []statField{
			{key: "num-pushed", label: "pushed", tip: "Packets pushed on to the decoder, in order."},
			{key: "num-lost", label: "lost", tip: "Packets the buffer gave up waiting for."},
			{key: "num-late", label: "late", tip: "Packets that arrived after their play time had passed."},
			{key: "num-duplicates", label: "duplicates", tip: "Packets that arrived more than once."},
			{key: "rtx-count", label: "rtx sent", tip: "Retransmit requests sent for missing packets."},
			{key: "rtx-success-count", label: "rtx recovered", tip: "Packets a retransmit request recovered."},
		},
	},
}

// statGroup reads one element's "stats" structure into labelled rows. It reports
// false while the element keeps no counters the table names, so a reader only
// grows a block once there is something in it.
func statGroup(src elementStats) (StatGroup, bool) {
	st, _ := src.element.ObjectProperty("stats").(*gst.Structure)
	if st == nil {
		return StatGroup{}, false
	}
	g := StatGroup{Name: src.element.GetName(), Tip: src.source.tip}
	for _, f := range src.source.fields {
		if !st.HasField(f.key) {
			continue
		}
		g.Rows = append(g.Rows, StatRow{
			Label: f.label,
			Tip:   f.tip,
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
