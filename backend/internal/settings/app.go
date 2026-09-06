package settings

// App is what the app does for itself, apart from any stream.
//
// Read on a schedule of this side's own rather than through an effect it is handed settings with,
// so a write holds as it is made and reaches the next start
// (api/proto/screenshare/v1/form.proto, FieldGroup.applied).
type App struct {
	// SendCrashReports lets a crash in an earlier run reach the relay operator on the next start,
	// as the bundle a manual report sends (internal/report).
	SendCrashReports bool `json:"sendCrashReports"`
	// CheckUpdatesOnStart reads the published release once per start,
	// which is what fills the update state with no press behind it (internal/update).
	CheckUpdatesOnStart bool `json:"checkUpdatesOnStart"`
	// TestStreams runs the synthetic publishers this machine exercises the viewing paths with,
	// off a fresh installation: an x264 encoder per slot runs for as long as the backend does.
	// The set converges on the write rather than at the next start (internal/app/teststreams.go).
	TestStreams bool `json:"testStreams"`
}
