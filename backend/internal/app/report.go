package app

import (
	"bytes"
	"errors"
	"path/filepath"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/report"
)

// sendReport builds one bundle and delivers it to the group service beside the stored relay
// (internal/report). SendReport and ReportLastCrash are the callers, one per kind.
// The stored settings name the relay: a report is about the deployment in use.
func (a *App) sendReport(kind string, include ...string) (string, error) {
	s := a.GetSettings()
	base, ok := s.Relay.GroupService()
	if !ok {
		return "", errors.New("the settings name no relay, and a report goes to the group service beside one. Name a relay first")
	}
	dir, err := ffmpeg.LogDir()
	if err != nil {
		return "", err
	}

	var bundle bytes.Buffer
	if err := report.Build(&bundle, report.Gather(a.version, kind), s, dir, include...); err != nil {
		return "", err
	}
	return a.groups.SendReport(base, &bundle)
}

// SendReport delivers a report a reader asked for, and answers the name it was stored under.
//
// Consent is the press, so App.SendsCrashReport is not read here.
// That setting covers the send nobody is standing in front of, which is the crash one.
//
// No marker and no gate on a repeat.
// A second press is a second report of its own moment,
// where a crash is one event and gets one report.
func (a *App) SendReport() (string, error) {
	id, err := a.sendReport(report.KindManual)
	if err != nil {
		logger.Warnf("the report did not go out: %v", err)
		return "", err
	}
	logger.Infof("sent report %s", id)
	return id, nil
}

// ReportLastCrash sends a report about the newest unreported crash
// among tag's earlier run logs, and nothing where every earlier run ended clean.
//
// Refused by the stored settings, which carry the whole of the consent behind an automatic send
// (settings.App.SendsCrashReport).
// An unput question refuses too, so nothing leaves before the shell has drawn it.
// The crash keeps its marker unwritten there,
// so answering and starting again sends what the refused run held back.
//
// Called once per start, off the startup path (cmd/backend).
// The marker keeps a crash to one report,
// and a send the network refused is tried again on the next start.
// Every failure is an Umgebungsfehler this process outlives: a report is a courtesy,
// and a machine that cannot send one still publishes.
func (a *App) ReportLastCrash(tag string) {
	assert.Assert(tag != "", "a crash is looked for under the run log tag")

	if !a.GetSettings().App.SendsCrashReport() {
		return
	}

	dir, err := ffmpeg.LogDir()
	if err != nil {
		logger.Warnf("not looking for a crash to report: %v", err)
		return
	}
	crashed, ok := report.UnreportedCrash(dir, tag, ffmpeg.OwnLogName())
	if !ok {
		return
	}

	id, err := a.sendReport(report.KindCrash, crashed)
	if err != nil {
		logger.Warnf("the last run crashed and its report did not go out: %v", err)
		return
	}
	if err := report.MarkReported(dir, filepath.Base(crashed)); err != nil {
		logger.Warnf("crash report %s went out unrecorded, so the next start may send it again: %v", id, err)
		return
	}
	logger.Infof("the last run crashed; sent report %s carrying %s", id, filepath.Base(crashed))
}
