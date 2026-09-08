package control

import "testing"

// The shell's gate reads the same variable (TrayIconHost.IsWanted), so the two agree per value.
func TestOnlyZeroHoldsTheIconOut(t *testing.T) {
	for _, tc := range []struct {
		set  string
		want bool
	}{
		{"0", true},
		{"", false},
		{"1", false},
		{"false", false},
	} {
		t.Setenv(EnvTray, tc.set)

		if got := trayForcedOff(); got != tc.want {
			t.Errorf("trayForcedOff() = %v with %s=%q, want %v", got, EnvTray, tc.set, tc.want)
		}
	}
}
