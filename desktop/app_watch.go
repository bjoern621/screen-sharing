package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/ffmpeg"
	"bjoernblessin.de/screenshare/relay"
	"bjoernblessin.de/screenshare/transport"
	"bjoernblessin.de/screenshare/watch"
)

// Live returns the relay snapshot. The frontend polls this every 2 seconds;
// per-path bitrates are only meaningful with such a steady poll interval.
func (a *App) Live() relay.Status {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	return a.relay.Fetch(s.RelayHost, s.ApiPort)
}

// WatchKey identifies one viewer window: a stream received over a specific
// transport. A stream can be watched over several transports at once (the relay
// re-serves each ingested stream on all its listeners), so the stream name
// alone is not a unique viewer identity.
type WatchKey struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
}

// Watching lists the viewers currently open, one entry per (stream, transport).
// Dead viewers are reaped here.
func (a *App) Watching() []WatchKey {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	keys := []WatchKey{}
	for key, watcher := range a.watchers {
		if watcher.Running() {
			keys = append(keys, key)
		} else {
			delete(a.watchers, key)
		}
	}
	return keys
}

// carriesStream reports whether the named transport can deliver the live stream,
// and the reason it cannot.
//
// The relay re-serves an ingested stream on the listeners whose protocol has a
// payload mapping for its bitstream, and on no others: MPEG-TS carries H.264 and
// H.265, so an SRT viewer opened on a VP9 or AV1 stream connects and receives
// nothing. Left to the viewer that reads as a broken stream, so the combination is
// refused here with the format named.
//
// A stream the relay does not report, or one whose tracks name no format this app
// knows, is not refused: the snapshot can be older than the stream, and refusing
// on absent information would block a viewer that would have worked.
func (a *App) carriesStream(streamName, transportName string) error {
	format := ""
	for _, path := range a.Live().Paths {
		if path.Name == streamName {
			format = path.Format
			break
		}
	}
	if format == "" {
		return nil
	}
	carried := transport.WatchNamesFor(capabilities.EngineFfmpeg, format)
	if slices.Contains(carried, transportName) {
		return nil
	}
	return fmt.Errorf("%s is %s, which %s cannot carry: watch it over %s",
		streamName, format, transportName, strings.Join(carried, " or "))
}

// StartWatch opens a viewer window for streamName, receiving it over
// transportName. The transport is chosen per viewer and is independent of the
// publish transport, so the same stream can be watched over any transport the
// relay serves it on. The watch package selects the viewer engine (ffplay by
// default, SCREENSHARE_VIEWER=mpv switches to mpv).
func (a *App) StartWatch(streamName, transportName string) error {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	if err := a.carriesStream(streamName, transportName); err != nil {
		return err
	}

	engine, err := watch.Select(transportName)
	if err != nil {
		return err
	}
	args, env, err := engine.Command(s, streamName, transportName)
	if err != nil {
		return err
	}

	a.procMu.Lock()
	defer a.procMu.Unlock()

	key := WatchKey{Name: streamName, Transport: transportName}
	watcher, present := a.watchers[key]
	if present && watcher.Running() {
		return fmt.Errorf("already watching %s over %s", streamName, transportName)
	}

	exe, err := ffmpeg.FindExe(engine.Exe())
	if err != nil {
		return err
	}

	// hideWindow must be false: SW_HIDE would hide the viewer's video window too.
	proc, err := ffmpeg.Start(exe, args, false, false, "watch-"+streamName+"-"+transportName, env, nil, nil,
		func(err error, stderrTail string, logPath string) {
			message := ""
			if err != nil {
				message = err.Error()
				if stderrTail != "" {
					message += "\n" + stderrTail
				}
				logger.Warnf("viewer for '%s' over %s failed: %v\n%s\nfull log: %s", streamName, transportName, err, stderrTail, logPath)
			} else {
				logger.Infof("viewer for '%s' over %s closed (log: %s)", streamName, transportName, logPath)
			}
			runtime.EventsEmit(a.ctx, "watch:exit", watchExitEvent{
				Name: streamName, Transport: transportName, Message: message, LogPath: logPath,
			})
		})
	if err != nil {
		return err
	}

	assert.IsNotNil(proc, "Start returns a non-nil Proc when err is nil")
	logger.Infof("watching '%s' over %s", streamName, transportName)
	a.watchers[key] = proc

	// Readiness is not signalled from here. The viewer is "connected" once the
	// relay reports a reader on the path, which the frontend already sees in its
	// Live() snapshot. That signal is independent of the window system, unlike a
	// probe for the ffplay window (no portable form exists under Wayland).
	return nil
}

func (a *App) StopWatch(streamName, transportName string) {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	key := WatchKey{Name: streamName, Transport: transportName}
	watcher, present := a.watchers[key]
	if present {
		watcher.Stop()
		delete(a.watchers, key)
		logger.Infof("stopped watching '%s' over %s", streamName, transportName)
	}
}
