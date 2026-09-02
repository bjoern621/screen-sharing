package settings

// Where the broadcast preview's picture is taken, as a stored value spells each route
// (docs/viewer-architecture.md, "What the broadcast preview draws").
//
// Off stands beside the two rather than under a control of its own:
// it closes the relay decode the end-to-end route holds and gives that reader slot back,
// so it is a route with an answer of "nowhere" rather than a way to blank a tile.

const (
	// PreviewOff draws nothing and decodes nothing. The publish is untouched.
	PreviewOff = "off"
	// PreviewLocal decodes the copy the publish child writes to a loopback port:
	// one decode here, no bandwidth and no reader counted at the relay.
	PreviewLocal = "local"
	// PreviewEndToEnd reads this machine's own stream back off the relay,
	// over the leg its viewer receives on:
	// a reader slot, a viewer among the figures beside the card, and one viewer's downlink.
	PreviewEndToEnd = "end-to-end"
)

// PreviewRoutes is every route a stored value may name, in the order a toggle draws them.
// By what each picture costs: nothing, one local decode, then a reader slot on the relay.
var PreviewRoutes = []string{PreviewOff, PreviewLocal, PreviewEndToEnd}

// ValidPreviewRoute reports whether name is a route this build draws.
// The file is the user's to edit, so a value no route carries is repaired rather than asserted
// (migrate.go).
func ValidPreviewRoute(name string) bool {
	for _, route := range PreviewRoutes {
		if route == name {
			return true
		}
	}
	return false
}
