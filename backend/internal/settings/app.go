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
	// CrashReportsAsked is the question about SendCrashReports having been put to the reader.
	// False on a fresh installation, and on a stored file written before the question.
	// The shell draws one dialog over the window while it is false and writes it on the way out.
	//
	// No form field: it records that a choice was offered, and no control writes it.
	CrashReportsAsked bool `json:"crashReportsAsked"`
	// CheckUpdatesOnStart reads the published release once per start,
	// which is what fills the update state with no press behind it (internal/update).
	CheckUpdatesOnStart bool `json:"checkUpdatesOnStart"`
	// TrayIcon puts the icon in the tray, which is what makes the window's close button a hide.
	// Off ends the app on that button, everything the menu offers being the window's own
	// (avalonia/README.md, "The tray").
	//
	// Read by the shell rather than here: the icon is drawn by the shell process.
	TrayIcon bool `json:"trayIcon"`
	// DiscordRichPresence has a share state itself on the Discord client running beside this app
	// (internal/discordrpc).
	// Read only while the group comes from Discord, which answers the channel and the audience,
	// so the press turning this on turns that on with it (internal/app, SaveSettings).
	//
	// No omitempty: a fresh installation carries it on,
	// so a stored off has to survive the decode Defaults seeds (migrate.go).
	DiscordRichPresence bool `json:"discordRichPresence"`
	// TestStreams runs the synthetic publishers this machine exercises the viewing paths with,
	// off a fresh installation: an x264 encoder per slot runs for as long as the backend does.
	// The set converges on the write rather than at the next start (internal/app/teststreams.go).
	TestStreams bool `json:"testStreams"`
}

// SendsCrashReport is a crash from an earlier run going out on this start.
//
// Two facts and not one: the question has to have been put and answered on.
// A fresh installation carries SendCrashReports on,
// so reading that alone would send the first crash of an install nobody had asked yet.
func (a App) SendsCrashReport() bool { return a.CrashReportsAsked && a.SendCrashReports }
