module bjoernblessin.de/screenshare

go 1.25.3

require (
	bjoernblessin.de/go-utils v1.0.1
	bjoernblessin.de/screenshare/api v0.0.0
	github.com/Microsoft/go-winio v0.6.2
	github.com/go-gst/go-gst v0.0.2
	github.com/godbus/dbus/v5 v5.1.0
	golang.org/x/sys v0.47.0
	google.golang.org/grpc v1.83.0
	google.golang.org/protobuf v1.36.11
)

// The control contract is a sibling module in this repository rather than a
// published one: it is generated from api/proto and versioned with the code that
// serves it, so it is reached by path and never by download.
replace bjoernblessin.de/screenshare/api => ./api

require (
	github.com/go-gst/go-glib v0.0.2 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
)

// replace github.com/wailsapp/wails/v2 v2.13.0 => C:\Users\bless\go\pkg\mod
