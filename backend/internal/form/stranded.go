package form

import (
	"slices"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// diagnosticsAboutStrandedControls names every control this combination leaves nothing to pick on.
//
// A control whose entries are all refused holds a value the same evaluation greys.
// The repair has nothing to walk to and leaves it standing (repair.go, legalOption),
// so the draft goes on naming a codec, an encoder or a leg,
// and a surface reading Field.value alone shows a settled choice nobody can act on.
// Without this the start button stays lit over a pipeline built from three refused values,
// and the failure arrives as the launch rather than as a sentence.
//
// Read off the resolved groups rather than evaluated again.
// The flags here are the ones a shell draws from,
// so the statement and the greyed list cannot disagree about what is left.
//
// A hidden or disabled control is passed over: its value reaches no pipeline,
// so nothing about it stops a stream (docs/field-availability.md).
//
// An empty list is a different fact and states none of this.
// It is a machine that enumerated nothing, no monitor or no open window,
// which each such field answers for itself, and which the repair reads as nothing to walk to.
func diagnosticsAboutStrandedControls(groups []*screensharev1.FieldGroup) []*screensharev1.Diagnostic {
	var out []*screensharev1.Diagnostic
	named := map[string]bool{}

	for _, g := range groups {
		severity := strandedSeverity(g.GetKey())
		for _, f := range g.GetFields() {
			if !f.GetVisible() || !f.GetEnabled() || len(f.GetOptions()) == 0 {
				continue
			}
			if slices.ContainsFunc(f.GetOptions(), func(o *screensharev1.FieldOption) bool {
				return o.GetEnabled()
			}) {
				continue
			}

			// The template, so the two rows of a repeated control state their gap once,
			// and so the key names a control every table keyed by one carries (keys.go).
			key := keyTemplate(f.GetKey())
			if named[key] {
				continue
			}
			named[key] = true
			out = append(out, diagnosticFor(severity, key, say(nothingLeftToPick, argOption(key))))
		}
	}
	return out
}

// strandedSeverity ranks a control with nothing left to pick by what reads it.
//
// A start is handed the groups marked publishReads, so a gap in one of them is a stream
// that cannot go out and the form says so by refusing the publish.
// Everywhere else it costs a tile rather than the stream,
// and a refusal would ground a working publish over the viewer's own gap.
func strandedSeverity(groupKey string) screensharev1.Severity {
	for _, g := range groups {
		if g.key != groupKey {
			continue
		}
		if g.publishReads {
			return screensharev1.Severity_SEVERITY_ERROR
		}
		return screensharev1.Severity_SEVERITY_WARNING
	}
	assert.Never("a resolved group is one the table declares", groupKey)
	return screensharev1.Severity_SEVERITY_UNSPECIFIED
}
