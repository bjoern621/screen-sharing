package settings

import "testing"

// A crash report goes out on the consent the reader gave, which is two facts and not one:
// whether the question was put, and what they answered.
// A fresh installation carries SendCrashReports on, so reading that alone would send
// the first crash of an install nobody had asked yet.
func TestACrashReportNeedsTheQuestionPutAndAnsweredOn(t *testing.T) {
	cases := []struct {
		name  string
		app   App
		sends bool
	}{
		{"asked and on", App{CrashReportsAsked: true, SendCrashReports: true}, true},
		{"asked and off", App{CrashReportsAsked: true, SendCrashReports: false}, false},
		{"unasked, on by default", App{CrashReportsAsked: false, SendCrashReports: true}, false},
		{"unasked and off", App{CrashReportsAsked: false, SendCrashReports: false}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.app.SendsCrashReport(); got != c.sends {
				t.Errorf("SendsCrashReport() = %v, want %v", got, c.sends)
			}
		})
	}
}

// The question stands unput on a fresh installation, which is what makes the shell draw it once.
func TestAFreshInstallationHasNotBeenAsked(t *testing.T) {
	if Defaults().App.CrashReportsAsked {
		t.Error("a fresh installation carries the crash report question as already put")
	}
}
