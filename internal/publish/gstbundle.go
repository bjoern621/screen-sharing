package publish

import (
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/gstbundle"
)

// FindGstExe locates the GStreamer launcher every pipeline in this package is run
// by: the publish engine's, the test streams' and the encode probe's.
//
// It is one function rather than a lookup per caller because the three spawn the
// same binary, and a bundle is only reachable to a caller that resolves it the way
// FindExe does. A bare name handed to exec.Command searches PATH alone, so the
// copy a Windows bundle ships beside the app - the one built against the plugins
// beside it - would be passed over for whatever the machine happens to carry, or
// for nothing at all.
func FindGstExe() (string, error) {
	return ffmpeg.FindExe(GstExe)
}

// GstChildEnv is what a spawned GStreamer child is given on top of this process's
// environment, empty where there is nothing to add.
//
// Where the plugins are is gstbundle's answer, because internal/receive asks the
// same question of the same installation. What differs is who is told: this app
// sets nothing on itself here, since a publish pipeline links no GStreamer and the
// variable belongs to the child that does.
func GstChildEnv() []string {
	path, ok := gstbundle.PluginPath()
	if !ok {
		return nil
	}
	return []string{gstbundle.PathVar + "=" + path}
}
