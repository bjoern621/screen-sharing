package gpu

import "testing"

// The vendor strings the drivers write, held to so a parse that stops reading one of them fails
// here rather than by leaving a driver-scoped gap unmatched on the machine it was written for.
func TestParseVendorStrings(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want Info
	}{
		{
			name: "mesa radeonsi",
			out:  "vainfo: Driver version: Mesa Gallium driver 26.1.6 for AMD Radeon 780M Graphics (radeonsi, phoenix, ACO, DRM 3.64, 7.1.5)",
			want: Info{
				Driver:  "radeonsi",
				Model:   "AMD Radeon 780M Graphics",
				Version: Version(26, 1, 6),
			},
		},
		{
			name: "two-field release",
			out:  "vainfo: Driver version: Mesa Gallium driver 25.3 for AMD Radeon RX 7900 XTX (radeonsi, navi31, ACO, DRM 3.61, 6.12.0)",
			want: Info{
				Driver:  "radeonsi",
				Model:   "AMD Radeon RX 7900 XTX",
				Version: Version(25, 3, 0),
			},
		},
		{
			name: "intel names no driver in its trailing list",
			out:  "vainfo: Driver version: Intel iHD driver for Intel(R) Gen Graphics - 24.1.0 ()",
			want: Info{Model: "Intel(R) Gen Graphics - 24.1.0", Version: Version(24, 1, 0)},
		},
		{
			name: "report carrying no vendor line",
			out:  "vainfo: VA-API version: 1.24 (libva 2.24.0)",
			want: Info{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parse(c.out)
			// Vendor is the whole string back and says nothing the other three do not.
			got.Vendor = ""
			if got != c.want {
				t.Fatalf("parsed %+v, table declares %+v", got, c.want)
			}
		})
	}
}

// A release packs into one figure that orders the way the releases do, which is the whole of what a
// fix bound asks of it.
func TestVersionOrders(t *testing.T) {
	ordered := []int{
		Version(25, 3, 0),
		Version(26, 0, 0),
		Version(26, 1, 5),
		Version(26, 1, 6),
		Version(26, 2, 0),
	}
	for i := 1; i < len(ordered); i++ {
		if ordered[i-1] >= ordered[i] {
			t.Fatalf("release %d packs to %d, which does not sort under its successor %d",
				i, ordered[i-1], ordered[i])
		}
	}
}
