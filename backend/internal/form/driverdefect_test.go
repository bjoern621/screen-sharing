package form

import (
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/rules"
)

// A control the machine's driver miscodes greys with the driver's own reason,
// and the same draft on a machine that named no driver keeps it.
//
// The whole path rather than the rule: the device reaches a resolve through Deps,
// and a fact that never arrives greys nothing
// while every table below it still says the right thing.
func TestADriverDefectGreysTheControlItWithholds(t *testing.T) {
	draft := availabilityDraft("portal", "av1_vaapi", "yuv420p", "rtsp")
	draft.Publish.Mode = capabilities.ModeCbr

	cases := []struct {
		name    string
		device  capabilities.Device
		enabled bool
	}{
		{
			name:    "the driver carrying the defect",
			device:  capabilities.Device{Driver: "radeonsi", Model: "AMD Radeon 780M Graphics"},
			enabled: false,
		},
		{name: "another driver", device: capabilities.Device{Driver: "iHD"}, enabled: true},
		{name: "no driver named", device: capabilities.Device{}, enabled: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := Deps{
				Platform: platform.Info{OS: "linux", Display: "wayland"},
				Device:   tc.device,
			}
			v := verdictsOf(deps, draft)

			if got := v.ValueEnabled(rules.AxisMode, capabilities.ModeCbr); got != tc.enabled {
				t.Fatalf("cbr enabled=%v, want %v", got, tc.enabled)
			}
			if tc.enabled {
				return
			}
			var named bool
			for _, reason := range v.ValueReasons(rules.AxisMode, capabilities.ModeCbr) {
				if reason.GetCode() == screensharev1.TextCode_TEXT_CODE_DRIVER_DEFECT_WITHHOLDS_OPTION {
					named = true
				}
			}
			if !named {
				t.Errorf("cbr greys without naming the driver, so the shell has nothing to show in its place")
			}
		})
	}
}
