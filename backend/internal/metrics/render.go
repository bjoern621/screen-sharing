// Package metrics is the group service's scrape:
// what it holds, rendered in the Prometheus text exposition format.
//
// The format is written here rather than taken from a client library.
// What is exported is families off state one process already holds,
// and a library brings a registry to keep them in,
// a second copy of facts the registry and the service own (docs/development-principles.md).
//
// Every gauge is derived from its owner when the request arrives,
// so nothing here has to be kept in step with a membership change,
// and a scrape that never comes costs nothing.
package metrics

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
)

// The two kinds this exports.
// A counter only ever rises and carries a _total suffix.
// A gauge is a reading that moves either way.
const (
	Gauge   = "gauge"
	Counter = "counter"
)

// Label is one dimension of a sample.
// Order is the caller's and is written as given.
type Label struct {
	Name  string
	Value string
}

// Sample is one reading, under the labels that name it.
type Sample struct {
	Labels []Label
	Value  float64
}

// Family is one metric and every sample of it in this scrape.
//
// A family with no samples is still written, so a consumer can tell a metric nothing holds from one
// this build does not export.
type Family struct {
	Name    string
	Help    string
	Type    string
	Samples []Sample
}

// Render writes families to w in the order given.
//
// Every failure is w's.
// The caller is an HTTP handler and the response is already partly written by the time one lands,
// so it is returned rather than turned into a page of its own.
func Render(w io.Writer, families []Family) error {
	assert.IsNotNil(w, "a scrape is rendered to somewhere")

	for _, family := range families {
		assert.Assert(family.Name != "", "a family is named")
		assert.Assert(family.Type == Gauge || family.Type == Counter, "a family is one of the kinds this exports", family.Name, family.Type)
		assert.Assert(family.Help != "", "a family says what it reads", family.Name)

		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", family.Name, escapeHelp(family.Help), family.Name, family.Type); err != nil {
			return err
		}
		for _, sample := range family.Samples {
			if _, err := fmt.Fprintf(w, "%s%s %s\n", family.Name, labels(sample.Labels), value(sample.Value)); err != nil {
				return err
			}
		}
	}
	return nil
}

// labels renders a sample's dimensions, and nothing at all where it carries none.
func labels(carried []Label) string {
	if len(carried) == 0 {
		return ""
	}

	var written strings.Builder
	written.WriteByte('{')
	for i, label := range carried {
		assert.Assert(label.Name != "", "a label is named")

		if i > 0 {
			written.WriteByte(',')
		}
		written.WriteString(label.Name)
		written.WriteString(`="`)
		written.WriteString(escapeValue(label.Value))
		written.WriteByte('"')
	}
	written.WriteByte('}')
	return written.String()
}

// value renders a reading in the shortest form that reads back as itself.
// 'g' would reach for an exponent on a figure a counter can plausibly hold,
// and a scrape carrying 1e+06 is one a reader has to decode.
func value(reading float64) string {
	return strconv.FormatFloat(reading, 'f', -1, 64)
}

// escapeValue escapes a label value.
// A display name arrives from whatever the member typed,
// so the three characters the format reserves are escaped rather than trusted.
var escapeValue = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace

// escapeHelp escapes help text, where a quote needs none and a newline would end the line early.
var escapeHelp = strings.NewReplacer(`\`, `\\`, "\n", `\n`).Replace
