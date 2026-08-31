package form

import (
	"testing"

	"bjoernblessin.de/screenshare/internal/rules"
)

// An axis that is a settings field carries that field's key, so a rule matching on the pixel format
// and the control it greys are one identifier on both sides.
//
// The two lists are spelled separately because the rules package is what every domain package
// registers into and may therefore import none of them, this one included.
// That makes them two copies of one fact, which is exactly what this repository does not allow
// to sit unwatched: a rename reaching only one of them would leave a rule matching an axis
// no control answers to, and the rule would quietly bind nothing rather than fail.
// Here it fails.
func TestAxesSpellTheFieldKeysTheyName(t *testing.T) {
	for _, pair := range []struct {
		axis string
		key  string
	}{
		{rules.AxisFormat, KeyFormat},
		{rules.AxisEncoder, KeyEncoder},
		{rules.AxisChroma, KeyChroma},
		{rules.AxisMode, KeyMode},
		{rules.AxisColorRange, KeyColorRange},
		{rules.AxisCapture, KeyCapture},
		{rules.AxisTransport, KeyTransport},
		{rules.AxisAudioCodec, KeyAudioCodec},
		{rules.AxisMemory, KeyCaptureMemory},
		{rules.AxisBitrateM, KeyBitrateM},
		{rules.AxisCq, KeyCq},
	} {
		if pair.axis != pair.key {
			t.Errorf("axis %q and field key %q are the same identifier", pair.axis, pair.key)
		}
	}
}

// Every axis that looks like a settings field is one the form declares.
// A derived fact carries no field key and is skipped by the prefix rather than by a list here,
// so a derived axis added to the vocabulary needs no edit in this test.
func TestEveryFieldAxisIsAFieldTheFormDeclares(t *testing.T) {
	declared := map[string]bool{}
	for _, f := range fieldTable {
		declared[f.key] = true
	}

	for _, axis := range rules.Axes() {
		if !isSettingsKey(axis.Name) {
			continue
		}
		if !declared[axis.Name] {
			t.Errorf("axis %q names a control the form does not declare", axis.Name)
		}
	}
}

// isSettingsKey reports whether an axis name is qualified the way a field key is:
// a settings group, a dot, then the field.
// A derived axis names a fact rather than a control,
// so it carries no group of the settings message.
func isSettingsKey(name string) bool {
	for _, group := range []string{"relay" + keySeparator, "publish" + keySeparator, "viewer" + keySeparator} {
		if len(name) > len(group) && name[:len(group)] == group {
			return true
		}
	}
	return false
}
