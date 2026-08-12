package receive

import (
	"github.com/go-gst/go-gst/pkg/gst"
)

// statSource names an element factory whose "stats" structure is worth reading, and
// the fields to take from it.
//
// It carries no labels and no explanations. Which counters a receiving pipeline can
// answer for is this side's knowledge, because it is the side that builds the
// pipeline; what a counter is called on screen is a reader's, because it is the side
// that has a width, a tone and a tooltip (api/proto/screenshare/v1/text.proto).
type statSource struct {
	factory string
	fields  []string
}

// statSources is the transport-side half of what a receiver reports. Which of these
// elements a pipeline holds is the transport's business: srtsrc counts an SRT link,
// the jitterbuffers inside rtspsrc count RTP. A field an element does not report is
// skipped, so a key this table has wrong costs its own row and nothing else.
var statSources = []statSource{
	{
		factory: "srtsrc",
		fields: []string{
			"packets-received",
			"packets-received-lost",
			"packets-received-retransmitted",
			"packets-received-dropped",
			"receive-rate-mbps",
			"bandwidth-mbps",
			"rtt-ms",
			"negotiated-latency-ms",
		},
	},
	{
		factory: "rtpjitterbuffer",
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

// statGroup reads one element's "stats" structure into keyed values. It reports false
// while the element keeps none of the counters the table names, so a reader only grows
// a block once there is something in it.
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

// statValue reads one stats field as a number, and reports false for a field whose
// type is none of the ones these elements count in. A value nothing can plot or print
// as a figure is left out rather than stringified: the row would say what the element
// called it and nothing about what it measures.
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
