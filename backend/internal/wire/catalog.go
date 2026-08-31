package wire

import (
	"slices"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/screensrc"
	"bjoernblessin.de/screenshare/internal/text"
	"bjoernblessin.de/screenshare/internal/transport"
)

// CatalogInput is the part of the catalog that is this machine's rather than the model's.
// The fixed tables are read through as package globals.
// Only what a probe or an enumeration answered travels in.
//
// The split is form.Deps's, for its reason: a table answers the same on every call,
// so a copy taken in would be a second definition of it waiting to go stale,
// while the monitors, the platform and the probe result differ per machine
// and a test builds a catalog for a machine it is not running on.
type CatalogInput struct {
	Platform platform.Info
	Monitors []display.Monitor
	Encoders encoders.Availability
	// AudioDevices is what this machine offers inside each audio kind, enumerated once
	// (internal/audiodev).
	// The machine half of the audio answer, the kinds beside it being the declared half.
	AudioDevices []platform.AudioDevice
}

// Catalog shapes every fixed fact onto the contract in one message.
//
// One message rather than one per table, the tables constraining each other:
// a shell that fetched them separately could hold a codec list from before a probe finished beside
// a probe result from after it.
//
// Nearly every assertion in this file sits here rather than at a caller.
// It turns closed internal enumerations (the engines, the legs, the GPU-path colour verdicts)
// into wire enums, and indexes tables by keys those same tables produced.
// A value outside a set fails at the conversion, where the offending row is named,
// rather than reaching a shell as an UNSPECIFIED enum that draws as nothing and explains nothing.
func Catalog(in CatalogInput) *screensharev1.Catalog {
	out := &screensharev1.Catalog{
		Platform: &screensharev1.Platform{Os: in.Platform.OS, Display: in.Platform.Display},
		Monitors: catalogMonitors(in.Monitors),

		Codecs:      catalogCodecs(),
		Decoders:    catalogDecoders(),
		AudioCodecs: catalogAudioCodecs(),
		Encoders:    catalogEncoders(in.Encoders),
		GpuPaths:    catalogGpuPaths(),
		Captures:    catalogCaptures(in.Platform),
		Carriage:    catalogCarriage(),

		Families:          slices.Clone(capabilities.Families),
		Modes:             slices.Clone(capabilities.Modes),
		FrameMemories:     slices.Clone(gpupath.Memories),
		CapabilityOptions: catalogOptions(),

		WatchTransports:         transport.WatchNames(capabilities.EngineFfmpeg),
		WatchTransportsByFormat: catalogWatchByFormat(),
		BrowserWatchTransports:  transport.WatchNames(transport.EngineBrowser),

		AudioSources: catalogAudioSources(in.Platform),
		AudioDevices: catalogAudioDevices(in.AudioDevices),

		NoMonitorPreview: catalogNoMonitorPreview(in.Platform),
	}

	// A machine may have no monitors this enumeration reached and no encoder the probe could run,
	// so neither is asserted.
	// The codecs, the capture backends and the carriage come from tables compiled into this binary,
	// so an empty one of those is a broken table rather than a bare machine.
	assert.Assert(len(out.GetCodecs()) > 0, "a catalog carries the codec table", len(out.GetCodecs()))
	assert.Assert(len(out.GetCaptures()) > 0, "a catalog carries the capture backends", len(out.GetCaptures()))
	assert.Assert(len(out.GetCarriage()) > 0, "a catalog carries what the transports carry", len(out.GetCarriage()))
	return out
}

// engines is the one table turning an engine's Go spelling into its wire enum.
// It covers the publish engines and the browser,
// the watch leg's reader that publishes nothing and states a carriage like the rest.
//
// The Go tables spell an engine as a string, the capabilities package depending on nothing so every
// consumer can read it from there.
// The contract carries an enum, a shell switching on a string being a shell holding the spelling.
// The two meet here and nowhere else.
var engines = map[string]screensharev1.Engine{
	capabilities.EngineFfmpeg: screensharev1.Engine_ENGINE_FFMPEG,
	capabilities.EngineGst:    screensharev1.Engine_ENGINE_GSTREAMER,
	transport.EngineBrowser:   screensharev1.Engine_ENGINE_BROWSER,
}

// The legs, spelled as transport.Register keys its carriage maps.
// The transport package declares no constant for them,
// so they are named once here rather than typed at each row catalogCarriage builds.
const (
	legPublish = "publish"
	legWatch   = "watch"
)

// legs is the same table for the two halves of a stream's path.
var legs = map[string]screensharev1.Leg{
	legPublish: screensharev1.Leg_LEG_PUBLISH,
	legWatch:   screensharev1.Leg_LEG_WATCH,
}

// colours is the same table for what a GPU path does to the colour the settings name.
// gpupath spells the verdicts exact and encoder (gpupath/colour.go),
// the contract PATH_COLOUR_EXACT and PATH_COLOUR_ENCODER.
var colours = map[gpupath.Colour]screensharev1.PathColour{
	gpupath.ColourExact:   screensharev1.PathColour_PATH_COLOUR_EXACT,
	gpupath.ColourEncoder: screensharev1.PathColour_PATH_COLOUR_ENCODER,
}

// engineEnum converts a named engine, publish or watch.
// One outside the table is a table that gained a value where this site did not,
// and it fails here rather than arriving as ENGINE_UNSPECIFIED to draw as an engine nobody named.
func engineEnum(engine string) screensharev1.Engine {
	e, ok := engines[engine]
	if !ok {
		assert.Never("an engine is one the tables above declare", engine)
	}
	return e
}

// gapEngine converts the engine a Gap names, where "" is a fact and not an omission:
// a gap naming no engine binds on every one, and the contract spells that ENGINE_ANY.
// Sharing ENGINE_UNSPECIFIED with it would make a field nobody set and a gap binding everywhere one
// value, and the dropped field the strongest claim the message can make.
// Every other value goes through engineEnum and is asserted there.
func gapEngine(engine string) screensharev1.Engine {
	if engine == "" {
		return screensharev1.Engine_ENGINE_ANY
	}
	return engineEnum(engine)
}

func legEnum(leg string) screensharev1.Leg {
	l, ok := legs[leg]
	if !ok {
		assert.Never("a carriage names one of the two legs", leg)
	}
	return l
}

func colourEnum(colour gpupath.Colour) screensharev1.PathColour {
	c, ok := colours[colour]
	if !ok {
		assert.Never("a GPU path states one of the two colour verdicts", string(colour))
	}
	return c
}

// catalogNoMonitorPreview states why this session cannot show what a monitor holds,
// and is nil where it can.
//
// Derived here rather than carried in, like the capture backends' availability beside it:
// the answer follows from the session and from a table this binary holds,
// so asking the machine would be reading one fact twice.
func catalogNoMonitorPreview(p platform.Info) *screensharev1.Text {
	_, gap := screensrc.Session(p)
	return gap
}

// catalogMonitors converts the display enumeration.
// A machine whose outputs could not be enumerated contributes an empty list,
// which a shell draws as "no monitor found" rather than as a monitor at index zero.
//
// A refresh rate of zero is one the enumeration could not read,
// and it crosses absent rather than as the number nought.
func catalogMonitors(monitors []display.Monitor) []*screensharev1.Monitor {
	out := make([]*screensharev1.Monitor, 0, len(monitors))
	for _, m := range monitors {
		monitor := &screensharev1.Monitor{
			Index:   int32(m.Index),
			Width:   int32(m.Width),
			Height:  int32(m.Height),
			OffsetX: int32(m.OffsetX),
			OffsetY: int32(m.OffsetY),
			Primary: m.Primary,
		}
		if m.RefreshHz > 0 {
			hz := int32(m.RefreshHz)
			monitor.RefreshHz = &hz
		}
		out = append(out, monitor)
	}
	return out
}

// catalogEncoders converts the probe result, keeping its two halves apart.
//
// The distinction is the point of the message:
// an engine whose own tooling is missing was asked about no codec,
// and an engine that was asked and found nothing usable is a machine without the hardware.
// Collapsing the first into the second presents a missing ffmpeg as a machine with no encoders,
// and leaves the encoders the probe assumes present selectable while certain to fail at launch.
//
// The halves are a oneof on one row per engine rather than two maps keyed by engine name:
// Detect answers an engine with verdicts or with the reason it could not be asked, never both,
// where parallel maps could hold the same engine twice and only an assertion here would catch it.
// The engines are walked in the capability table's order,
// so two catalogs built from one availability compare equal.
func catalogEncoders(a encoders.Availability) *screensharev1.EncoderAvailability {
	out := &screensharev1.EncoderAvailability{}
	for _, engine := range capabilities.Engines {
		perCodec, probed := a.Usable[engine]
		reason, unprobed := a.Unprobed[engine]
		assert.Assert(!(probed && unprobed), "an engine is either probed or carries the reason it was not", engine)
		if !probed && !unprobed {
			// An engine nothing has asked about contributes no row, which is what a fresh process looks
			// like: nothing is claimed about an engine until a probe has reached it.
			continue
		}

		row := &screensharev1.EngineProbe{Engine: engineEnum(engine)}
		if unprobed {
			// The tool's own "not found" is the machine's answer rather than a fact about the domain,
			// so it stays in the run log and what crosses is that this engine could not be asked at all.
			assert.Assert(reason != "", "an unprobed engine records why it could not be asked", engine)
			row.Result = &screensharev1.EngineProbe_Unprobed{
				Unprobed: text.Of(screensharev1.TextCode_TEXT_CODE_ENGINE_TOOLING_MISSING,
					text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_ENGINE, engine)),
			}
		} else {
			usable := make(map[string]bool, len(perCodec))
			for codec, ok := range perCodec {
				usable[codec] = ok
			}
			row.Result = &screensharev1.EngineProbe_Probed{
				Probed: &screensharev1.EngineCodecs{Usable: usable},
			}
		}
		out.Engines = append(out.Engines, row)
	}
	return out
}

// catalogGpuPaths converts the pair table:
// the capture backend and encoder family combinations whose frames reach the encoder without
// leaving the GPU.
//
// The Signalled triple a ColourEncoder row carries has no field of its own.
// It is what the row's cost statement is about, so it crosses as that statement's arguments,
// and nothing else on this table needs it.
func catalogGpuPaths() []*screensharev1.GpuPath {
	out := make([]*screensharev1.GpuPath, 0, len(gpupath.Paths))
	for _, p := range gpupath.Paths {
		out = append(out, &screensharev1.GpuPath{
			Engine:  engineEnum(p.Engine),
			Capture: p.Capture,
			Family:  p.Family,
			Import:  gpuPathImport(p),
			Colour:  colourEnum(p.Colour),
			Cost:    GpuPathCost(p),
		})
	}
	return out
}

// gpuPathImport names what carries the frames across one pair's device path.
func gpuPathImport(p gpupath.Path) *screensharev1.Text {
	assert.Assert(p.Import != screensharev1.TextCode_TEXT_CODE_UNSPECIFIED,
		"a device path names what carries its frames", p.Capture, p.Family)
	return text.Of(p.Import)
}

// GpuPathCost states what a colour-trading pair takes instead of the settings' colour,
// and nil for a pair that takes nothing.
//
// The pixel format and colour range the encoder signals ride along, being the trade itself:
// the run publishes something other than what the two colour fields show,
// and a statement naming the trade without the values would leave the reader guessing which.
// Exported because the form makes the same statement on the greyed controls,
// and one function keeps the reference set and the form from describing one row two ways.
func GpuPathCost(p gpupath.Path) *screensharev1.Text {
	if !p.Colour.TradesColour() {
		return nil
	}
	assert.Assert(p.Cost != screensharev1.TextCode_TEXT_CODE_UNSPECIFIED,
		"a colour-trading device path names what it takes", p.Capture, p.Family)
	return text.Of(p.Cost,
		text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_CHROMA, p.Signalled.Chroma),
		text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_COLOR_RANGE, p.Signalled.Range))
}

// catalogCaptures converts the publish registry: one row per capture backend the app can run,
// with the engine that runs it and the publish legs that engine can carry.
//
// available and reason are read through to publish.Available, where the platform and session gate
// lives (docs/capture-architecture.md, "The backend's platform applicability").
// It sits beside the registry: a backend that cannot run on this machine and a backend with no
// engine are one question asked twice,
// and the sentence a shell shows has to be the one the form greys the control with,
// so restating the rule here would be a second copy of it.
func catalogCaptures(p platform.Info) []*screensharev1.CaptureBackend {
	names := publish.Captures()
	out := make([]*screensharev1.CaptureBackend, 0, len(names))
	for _, name := range names {
		engine, err := publish.EngineFor(name)
		// Captures lists the registry's own keys, so every name here resolves.
		// Skipping one would leave a capture backend out of the catalog,
		// and a capture the catalog does not name is one no shell can offer at all.
		assert.IsNil(err, "a listed capture backend has a publisher", name)
		transports, err := publish.TransportsFor(name)
		assert.IsNil(err, "a listed capture backend has a publisher", name)

		available, reason := publish.Available(name, p)
		assert.Assert(available || reason != nil, "an unavailable capture backend says why", name)

		out = append(out, &screensharev1.CaptureBackend{
			Name:       name,
			Engine:     engineEnum(engine),
			Transports: transports,
			Available:  available,
			Reason:     reason,
			// A privilege a backend needs is no unavailability, so it rides beside the verdict rather than
			// inside it: the process holds it or the capture dies at launch,
			// and nothing here can tell which in advance.
			Grant: publish.Grant(name),
		})
	}
	return out
}

// catalogAudioSources narrows the second-track source table to the sources a session of this
// platform serves, which is what the field claims.
//
// Which sources exist is the platform's answer,
// so the rows are read from there rather than stated here
// (docs/domain-model.md, "The second-track capture sources").
// The table answers for every declared source on every platform,
// and its two readers want opposite halves of that answer:
// a catalog names what the machine has, and the form offers all of them and greys what it does not,
// a source greyed with what the machine is missing teaching where a shorter list only puzzles.
// Neither derives its list from the other, and the rule either would restate lives in one file.
//
// No availability flag rides along, unlike the capture backends.
// A capture backend crosses as a message with room for one, this field being a repeated string.
// The reason a source is out of reach is a sentence a screen shows,
// which is the form's half of the contract and arrives on FieldOption.
func catalogAudioSources(p platform.Info) []string {
	sources := platform.AudioSources(p)
	out := make([]string, 0, len(sources))
	for _, s := range sources {
		if !s.Available {
			continue
		}
		out = append(out, s.ID)
	}

	// Every platform serves the absent source.
	// A machine offering none is a table that stopped declaring one rather than a bare machine.
	assert.Assert(len(out) > 0, "a catalog carries the second-track sources this platform serves", p.OS)
	return out
}

// catalogCarriage converts the transport tables: one row per transport, leg and engine that exists.
//
// A leg an engine cannot serialize contributes no row,
// keeping the wire shape and the registry's own invariant one statement:
// Register holds a stated carriage and the matching serialization capability to each other,
// so a row exists exactly where there is code to build that leg on that engine.
func catalogCarriage() []*screensharev1.TransportCarriage {
	var out []*screensharev1.TransportCarriage
	for _, name := range transport.Names() {
		f, ok := transport.FormatsOf(name)
		assert.Assert(ok, "a listed transport is a registered one", name)
		// Each leg is walked over its own engine list, the readers not being the publishers:
		// the browser reads a page the relay serves and publishes nothing,
		// so one walk over either list drops rows.
		for _, engine := range capabilities.Engines {
			if c, ok := f.Publish[engine]; ok {
				out = append(out, carriageRow(name, legPublish, engine, c))
			}
		}
		for _, engine := range transport.WatchEngines {
			if c, ok := f.Watch[engine]; ok {
				out = append(out, carriageRow(name, legWatch, engine, c))
			}
		}
	}
	return out
}

// carriageRow is one (transport, leg, engine) row.
// Video formats and audio codecs travel together:
// what a listener carries as a bitstream and what it carries as a second track are one fact about
// that listener.
func carriageRow(name, leg, engine string, c transport.Carriage) *screensharev1.TransportCarriage {
	assert.Assert(len(c.Video) > 0, "a stated carriage carries a video format", name, leg, engine)
	return &screensharev1.TransportCarriage{
		Name:   name,
		Leg:    legEnum(leg),
		Engine: engineEnum(engine),
		Video:  slices.Clone(c.Video),
		Audio:  slices.Clone(c.Audio),
	}
}

// catalogWatchByFormat narrows the watch list per bitstream format.
//
// The relay re-serves an ingested stream on the listeners whose protocol has a payload mapping
// for it and on no others, so the watch choice is per stream rather than global.
// The whole list would put an SRT viewer in front of a VP9 stream, which MPEG-TS has no mapping
// for, and the viewer would open, receive nothing and report a broken stream instead
// of an impossible combination.
//
// The engine is the URL-opening players', this being the list an external viewer is opened from.
// What a receiving pipeline can take is the GStreamer watch rows of carriage,
// where a viewer that is not a player reads it.
func catalogWatchByFormat() map[string]*screensharev1.TransportList {
	formats := capabilities.Formats()
	out := make(map[string]*screensharev1.TransportList, len(formats))
	for _, format := range formats {
		out[format] = &screensharev1.TransportList{
			Transports: transport.WatchNamesFor(capabilities.EngineFfmpeg, format),
		}
	}
	return out
}

// catalogAudioDevices carries the enumeration across in the order it was taken,
// the order the sound server reported and the order a control offers.
func catalogAudioDevices(devices []platform.AudioDevice) []*screensharev1.AudioDevice {
	out := make([]*screensharev1.AudioDevice, 0, len(devices))
	for _, d := range devices {
		assert.Assert(d.Kind != "", "an enumerated audio device names the kind it is inside", d.ID)
		assert.Assert(d.ID != "", "an enumerated audio device carries the handle it is opened by", d.Kind)
		out = append(out, &screensharev1.AudioDevice{Kind: d.Kind, Id: d.ID, Name: d.Name})
	}
	return out
}
