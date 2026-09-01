package portal

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// propsIface reads a D-Bus object's properties.
const propsIface = "org.freedesktop.DBus.Properties"

// Capabilities is what the ScreenCast interface on this machine serves.
//
// The compositor's backend fills the two masks,
// so the answer belongs to the desktop rather than to the interface:
// a mode wlroots leaves out is one KWin serves through the same call.
// xdg-desktop-portal validates SelectSources against them and refuses the whole call for anything
// outside, so a mask read in advance is the difference between a greyed option and a publish
// that does not start.
type Capabilities struct {
	// CursorModes is the portal's own bitmask, CursorHidden|CursorEmbedded|CursorMetadata.
	// 0 is a portal nothing asked, which withholds nothing.
	CursorModes CursorMode
	// SourceTypes is the portal's own bitmask, SourceMonitor|SourceWindow|SourceVirtual.
	// 0 reads as CursorModes' 0 does.
	SourceTypes SourceType
}

// ServesCursor reports whether the portal offers mode.
// An unread mask offers every one: nothing was asked here, so the SelectSources refusal
// stays the answer.
func (c Capabilities) ServesCursor(mode CursorMode) bool {
	assert.Assert(mode != 0, "a pointer question names a cursor mode")

	if c.CursorModes == 0 {
		return true
	}
	return c.CursorModes&mode != 0
}

// SourcesServed is types narrowed to what the portal offers, types unchanged where no mask was read.
// 0 where the portal serves none of them, which is a picker there is nothing to ask for.
func (c Capabilities) SourcesServed(types SourceType) SourceType {
	assert.Assert(types != 0, "a narrowing names the source kinds asked for")

	if c.SourceTypes == 0 {
		return types
	}
	return types & c.SourceTypes
}

// Detect reads the ScreenCast capability properties off the session bus.
//
// Every failure belongs to the desktop: no session bus, no portal, a portal answering neither
// property.
// Umgebungsfehler, and the zero Capabilities withholds nothing.
func Detect(ctx context.Context) (Capabilities, error) {
	assert.IsNotNil(ctx, "a capability read runs under a context")

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return Capabilities{}, fmt.Errorf("connect session bus: %w", err)
	}
	defer conn.Close()

	return readCapabilities(ctx, conn)
}

// readCapabilities is one GetAll on the ScreenCast interface.
// A property the portal omits stays 0, the reading a portal nothing asked leaves.
func readCapabilities(ctx context.Context, conn *dbus.Conn) (Capabilities, error) {
	assert.IsNotNil(ctx, "a capability read runs under a context")
	assert.IsNotNil(conn, "a capability read is made over a bus connection")

	var props map[string]dbus.Variant
	call := conn.Object(service, objectDir).CallWithContext(ctx, propsIface+".GetAll", 0, scIface)
	if err := call.Store(&props); err != nil {
		return Capabilities{}, fmt.Errorf("read %s properties: %w", scIface, err)
	}
	cursors, _ := props["AvailableCursorModes"].Value().(uint32)
	sources, _ := props["AvailableSourceTypes"].Value().(uint32)
	return Capabilities{CursorModes: CursorMode(cursors), SourceTypes: SourceType(sources)}, nil
}

var (
	once   sync.Once
	cached Capabilities
)

// probeTimeout bounds the property read.
// A portal that is running answers in milliseconds, so the bound catches a wedged bus alone,
// and what waits behind it is every form resolve this process serves.
const probeTimeout = 3 * time.Second

// Cached is Detect's answer, taken on the first call and read from memory after it.
//
// One D-Bus round trip rather than a subprocess,
// so a form resolve takes it directly and there is no waiting form of it (internal/audiodev, Cached).
// Held for the process lifetime: a portal replaced underneath keeps its old answer until this
// process is replaced too, which costs a greying and no running stream.
func Cached(ctx context.Context) Capabilities {
	assert.IsNotNil(ctx, "a capability read runs under a context")

	once.Do(func() {
		// xdg-desktop-portal is a Linux desktop's, so nothing is asked elsewhere.
		if runtime.GOOS != "linux" {
			return
		}
		ctx, cancel := context.WithTimeout(ctx, probeTimeout)
		defer cancel()

		caps, err := Detect(ctx)
		if err != nil {
			// The zero mask withholds nothing, so every mode stays on offer and SelectSources
			// answers for them, as on a machine nothing asked.
			logger.Warnf("desktop portal capabilities not read, its options stay on offer: %v", err)
			return
		}
		cached = caps
	})
	return cached
}
