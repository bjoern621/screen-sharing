package receive

import (
	"github.com/go-gst/go-gst/pkg/gst"
)

// statSource names an element factory whose "stats" structure is read, and the fields taken off it.
//
// No labels and no explanations.
// Which counters a receiving pipeline can answer for is this side's knowledge, this side building
// the pipeline.
// What a counter is called on screen is a reader's, that side having a width, a tone and a tooltip
// (api/proto/screenshare/v1/text.proto).
type statSource struct {
	factory string
	fields  []string
}

// statSources is the transport-side half of what a receiver reports.
// Which of these elements a pipeline holds is the watch leg's business:
// srtsrc counts an SRT link, the jitterbuffers inside rtspsrc count RTP.
// A field an element does not report is skipped,
// so a key this table has wrong costs its own row and nothing else.
var statSources = []statSource{
	{
		factory: "srtsrc",
		fields: []string{
			// Cumulative over the link.
			"packets-received",
			"packets-received-lost",
			"packets-received-retransmitted",
			"packets-received-dropped",
			// Mbit/s, a rate rather than a total:
			// what is arriving, and SRT's estimate of what the link carries.
			"receive-rate-mbps",
			"bandwidth-mbps",
			// ms.
			"rtt-ms",
			// ms, the SRT window both ends settled on.
			// MediaMTX floors every SRT hop at 120 and has no config key that lowers it,
			// so a latency setting under 120 changes nothing and reads back here as 120.
			"negotiated-latency-ms",
		},
	},
	{
		factory: "rtpjitterbuffer",
		// Cumulative over the buffer's life.
		fields: []string{
			"num-pushed",
			"num-lost",
			"num-late",
			"num-duplicates",
			"rtx-count",
			"rtx-success-count",
		},
	},
}

// statGroup reads one element's "stats" structure into keyed values.
// False while the element keeps none of the counters the table names,
// so a reader grows a block only once there is something in it.
func statGroup(src elementStats) (StatGroup, bool) {
	st, _ := src.element.ObjectProperty("stats").(*gst.Structure)
	if st == nil {
		return StatGroup{}, false
	}
	g := StatGroup{Factory: src.source.factory, Element: src.element.GetName()}
	for _, key := range src.source.fields {
		if !st.HasField(key) {
			continue
		}
		value, ok := statValue(st.GetValue(key))
		if !ok {
			continue
		}
		g.Values = append(g.Values, StatValue{Key: key, Value: value})
	}
	return g, len(g.Values) > 0
}

// statValue reads one stats field as a number,
// and reports false for a type none of these elements counts in.
// A value nothing can plot or print as a figure is left out rather than stringified:
// the row would carry what the element called it and nothing about what it measures.
func statValue(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
}
