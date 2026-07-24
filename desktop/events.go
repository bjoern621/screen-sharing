package main

// exitEvent is the payload of the "publish:exit" event: the (possibly empty)
// error message and the path of the full run log.
type exitEvent struct {
	Message string `json:"message"`
	LogPath string `json:"logPath"`
}

// watchExitEvent is the payload of the "watch:exit" event. Name and Transport
// together identify which viewer exited, so the UI clears the connecting state
// of the right (stream, transport) rather than every viewer of the stream.
type watchExitEvent struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Message   string `json:"message"`
	LogPath   string `json:"logPath"`
}
