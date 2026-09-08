package control

import "os"

// EnvTray holds the icon out of the tray for a whole run, whatever the app setting holds.
// The shell reads it to decide whether an icon stands (avalonia/README.md, "The tray"),
// and this process reads it to grey the setting behind it:
// one variable, and the two halves of the app agreeing about what it did.
const EnvTray = "MIRRORME_TRAY"

// trayForcedOff reports whether the environment refuses the icon.
// "0" is off and every other value, absence included, leaves the setting deciding.
func trayForcedOff() bool {
	return os.Getenv(EnvTray) == "0"
}
