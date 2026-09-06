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

// SendReport bundles this machine's facts and run logs
// and delivers them to the group service beside the stored relay (internal/report).
// The manual half of the pair, asked for from a shell; ReportLastCrash sends the other.
func (a *App) SendReport() (string, error) {
	return a.sendReport(report.KindManual)
}

// sendReport builds one bundle and delivers it.
// The stored settings name the relay: a report is about the deployment in use,
// and no draft exists where the ask comes from (control.proto, SendReport).
func (a *App) sendReport(kind string, include ...string) (string, error) {
	s := a.GetSettings()
	base, ok := s.Relay.GroupService()
	if !ok {
		return "", errors.New("a report goes to the relay's group service, and the settings name no relay")
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

// ReportLastCrash sends a report about the newest unreported crash
// among tag's earlier run logs, and nothing where every earlier run ended clean.
//
// Refused by the stored settings, which is the whole of the consent behind an automatic send
// (settings.App.SendCrashReports).
// The crash keeps its marker unwritten there,
// so turning the setting on and starting again sends what the refused run held back.
//
// Called once per start, off the startup path (cmd/backend).
// The marker keeps a crash to one report,
// and a send the network refused is tried again on the next start.
// Every failure is an Umgebungsfehler this process outlives: a report is a courtesy,
// and a machine that cannot send one still publishes.
func (a *App) ReportLastCrash(tag string) {
	assert.Assert(tag != "", "a crash is looked for under the run log tag")

	if !a.GetSettings().App.SendCrashReports {
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
