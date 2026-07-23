package main

// exitEvent is the payload of the "publish:exit" event: the (possibly empty)
// error message and the path of the full run log.
type exitEvent struct {
	Message string `json:"message"`
	LogPath string `json:"logPath"`
}

// watchExitEvent is the payload of the "watch:exit" event. Name lets the UI
// clear the right stream's connecting state.
type watchExitEvent struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	LogPath string `json:"logPath"`
}
