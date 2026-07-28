package main

// exitEvent is the payload of the "publish:exit", "teststream:exit" and
// "nativegrid:exit" events: the (possibly empty) error message and the path of
// the full run log.
type exitEvent struct {
	Message string `json:"message"`
	LogPath string `json:"logPath"`
}

// publishStateEvent is the payload of the "publish:state" event: whether the app
// is publishing now. It goes out on every change, whoever made it, which is what
// keeps a window that did not ask for one from missing it.
type publishStateEvent struct {
	Publishing bool `json:"publishing"`
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
