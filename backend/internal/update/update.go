// Package update carries one install from the release it is running to the one published beside it.
//
// Three questions, answered in that order.
// Whether this copy asks the release service anything, which the environment and the build stamp
// decide.
// Whether it replaces its own files, which the channel decides (channel.go).
// And what is on disk waiting for a restart, which the staging directory holds (stage.go).
//
// What is asked of the network is the project's own published release and nothing else
// (internal/release).
// What is put on disk is one file, checked against the hash the release records,
// and executed once it verifies.
//
// Every failure here is an Umgebungsfehler.
// A machine with no route to the forge runs unchanged and is told so,
// and a download that stopped leaves the running build exactly where it was.
package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/release"
	"bjoernblessin.de/screenshare/internal/text"
)

// Stage is how far this install has got towards the published release.
// The contract's UpdateStage, in this package's own vocabulary.
type Stage int

const (
	StageOff Stage = iota
	StageUnchecked
	StageChecking
	StageCurrent
	StageAvailable
	StageFetching
	StageReady
	StageFailed
)

// State is the whole answer, and what crosses as UpdateState.
type State struct {
	Stage   Stage
	Running string
	Latest  string
	Page    string
	Percent int
	// Why nothing is asked, and why nothing is installed. Both the channel's answer.
	Unchecked     *screensharev1.Text
	Uninstallable *screensharev1.Text
	// What went wrong, with the failing side's own words beside it.
	Failure *screensharev1.Text
	Detail  string
}

// LaunchName is the shell binary an install comes back as, beside the backend in one directory
// (packaging/linux/package.sh).
const LaunchName = "mirrorme"

// Manager holds what one install knows about the release published beside it.
//
// One per process, so every shell reads the same answer.
// A check one of them asked for reaches the rest on the event stream.
type Manager struct {
	version string
	offer   Offer
	// target holds the running app, launch is the binary a finished install starts.
	target string
	launch string
	// announce hands a whole state to whatever puts it on the wire.
	announce func(State)

	mu sync.Mutex
	// stage and what fills it in, guarded together: a check writes the lot on each step.
	stage   Stage
	latest  string
	page    string
	percent int
	failure *screensharev1.Text
	detail  string
	// pending is the staged release, empty while nothing is staged.
	pending Pending
	// checking is a check already running, which a second press joins rather than duplicates.
	checking bool
}

// New reads the environment and the build stamp once, and answers what this install may do.
//
// exe is the running backend binary, whose directory is the install this app would replace.
// announce is called with a whole state on every change, on whichever goroutine made it.
func New(version string, stamped Channel, exe string, announce func(State)) *Manager {
	assert.Assert(version != "", "an install states which build it is running")
	assert.Assert(exe != "", "an install names the binary it is running from")
	assert.IsNotNil(announce, "an install announces what it learns")

	target := filepath.Dir(exe)
	launch := filepath.Join(target, LaunchName)
	if runtime.GOOS == "windows" {
		launch += ".exe"
	}

	m := &Manager{
		version:  version,
		offer:    Decide(Resolve(stamped, target, os.Getenv(EnvFlatpak)), version, os.Getenv(EnvCheck)),
		target:   target,
		launch:   launch,
		announce: announce,
	}

	m.stage = StageUnchecked
	if m.offer.Unchecked != nil {
		m.stage = StageOff
	}

	dir, err := StageDir()
	if err != nil {
		// Umgebungsfehler: nothing stages, and a check says so when one is asked for.
		logger.Warnf("no update directory, so no release stages: %v", err)
		return m
	}

	if m.stage == StageOff {
		// An install that asks nothing installs nothing, so a download an earlier run left
		// is disk nobody reclaims. Cleared rather than kept for a state that cannot reach it.
		if err := ClearStage(dir); err != nil {
			logger.Warnf("cannot clear %s: %v", dir, err)
		}
		return m
	}

	// A release staged by an earlier run is still staged, and a restart is still what installs it.
	if pending, staged := ReadPending(dir); staged {
		m.pending = pending
		m.latest = pending.Tag
		m.stage = StageReady
	}
	return m
}

// State answers what this install knows, without reaching anything.
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.stateLocked()
}

func (m *Manager) stateLocked() State {
	return State{
		Stage:         m.stage,
		Running:       m.version,
		Latest:        m.latest,
		Page:          m.page,
		Percent:       m.percent,
		Unchecked:     m.offer.Unchecked,
		Uninstallable: m.offer.Uninstallable,
		Failure:       m.failure,
		Detail:        m.detail,
	}
}

// CheckLimit bounds a whole check, the download included.
//
// The work outlives the call that asked for it, so nothing else would ever end it:
// a transfer that stalls at ninety percent would otherwise hold a connection for the process's life.
const CheckLimit = 30 * time.Minute

// Check reads the published release, and fetches it where this install replaces its own files.
//
// Returns as soon as the work is under way, every step of it reaching the caller's announce.
// Idempotent: a check already running is joined rather than started again,
// and a release already staged and verified is not fetched twice.
//
// It takes no context, as StartMonitorPreview takes none, and for the same reason:
// the work outlives the call, so a caller's context would cancel a download
// the moment the reply that started it was written.
//
// Refused where the install asks nothing at all, which a caller reads off State first.
func (m *Manager) Check() error {
	m.mu.Lock()
	if m.offer.Unchecked != nil {
		m.mu.Unlock()
		return fmt.Errorf("this install does not check for updates")
	}
	if m.checking {
		m.mu.Unlock()
		return nil
	}

	m.checking = true
	m.stage, m.failure, m.detail = StageChecking, nil, ""
	state := m.stateLocked()
	m.mu.Unlock()

	m.announce(state)
	go m.run()
	return nil
}

// run is the check itself: read the release, compare, and fetch what the comparison found.
func (m *Manager) run() {
	ctx, cancel := context.WithTimeout(context.Background(), CheckLimit)
	defer cancel()

	defer func() {
		m.mu.Lock()
		m.checking = false
		m.mu.Unlock()
	}()

	latest, err := release.Fetch(ctx)
	if err != nil {
		logger.Warnf("the published release could not be read: %v", err)
		m.fail(screensharev1.TextCode_TEXT_CODE_UPDATE_SERVICE_UNREADABLE, err.Error())
		return
	}

	m.mu.Lock()
	m.latest, m.page = latest.Tag, latest.URL
	m.mu.Unlock()

	if release.Compare(m.version, latest.Tag) != release.StateBehind {
		m.settle(StageCurrent)
		return
	}
	if m.offer.Uninstallable != nil {
		// The release is worth naming and the files belong to somebody else.
		m.settle(StageAvailable)
		return
	}

	m.settle(StageAvailable)
	m.fetch(ctx, latest)
}

// fetch downloads the release and records it as the one a restart installs.
func (m *Manager) fetch(ctx context.Context, latest release.Latest) {
	name := AssetName(m.offer.Asset, latest.Tag)

	asset, attached := latest.Named(name)
	if !attached {
		logger.Warnf("release %s attaches no %s", latest.Tag, name)
		m.failText(text.Of(screensharev1.TextCode_TEXT_CODE_UPDATE_NO_DOWNLOAD,
			text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_CHANNEL, string(m.offer.Channel)),
			text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_VERSION, latest.Tag)), name)
		return
	}
	if !Verifiable(asset) {
		logger.Warnf("release %s records no hash for %s", latest.Tag, name)
		m.failText(text.Of(screensharev1.TextCode_TEXT_CODE_UPDATE_DOWNLOAD_UNVERIFIABLE,
			text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_VERSION, latest.Tag)), name)
		return
	}

	m.mu.Lock()
	staged := m.pending.Tag == latest.Tag
	m.mu.Unlock()
	if staged {
		// Already on disk and already checked, so the restart is all that is left.
		m.settle(StageReady)
		return
	}

	dir, err := StageDir()
	if err != nil {
		m.fail(screensharev1.TextCode_TEXT_CODE_UPDATE_DOWNLOAD_FAILED, err.Error())
		return
	}
	if err := ClearStage(dir); err != nil {
		m.fail(screensharev1.TextCode_TEXT_CODE_UPDATE_DOWNLOAD_FAILED, err.Error())
		return
	}

	m.step(StageFetching, 0)
	path, err := Fetch(ctx, asset, dir, func(percent int) { m.step(StageFetching, percent) })
	if err != nil {
		logger.Warnf("downloading %s: %v", name, err)
		code := screensharev1.TextCode_TEXT_CODE_UPDATE_DOWNLOAD_FAILED
		if errors.Is(err, ErrCorrupt) {
			code = screensharev1.TextCode_TEXT_CODE_UPDATE_DOWNLOAD_CORRUPT
		}
		m.fail(code, err.Error())
		return
	}

	pending := Pending{
		Tag:    latest.Tag,
		File:   path,
		Method: m.offer.Method,
		Target: m.target,
		Launch: m.launch,
	}
	if err := WritePending(dir, pending); err != nil {
		m.fail(screensharev1.TextCode_TEXT_CODE_UPDATE_DOWNLOAD_FAILED, err.Error())
		return
	}

	m.mu.Lock()
	m.pending = pending
	m.mu.Unlock()

	logger.Infof("release %s is staged at %s and installs on the next start", latest.Tag, path)
	m.settle(StageReady)
}

// Install starts the staged release and leaves the running app to close.
//
// The applier is a process of its own and outlives both halves of the app:
// it waits for this process to exit, puts the files in place and starts the app again.
// A copy of this binary runs it, so the tree it replaces holds nothing it is reading from.
//
// Refused below StageReady: nothing is installed that has not been fetched and verified.
func (m *Manager) Install() error {
	m.mu.Lock()
	pending, ready := m.pending, m.stage == StageReady
	m.mu.Unlock()

	if !ready || pending.File == "" {
		return fmt.Errorf("no release is staged")
	}

	applier, err := applierCopy(filepath.Dir(pending.File))
	if err != nil {
		m.fail(screensharev1.TextCode_TEXT_CODE_UPDATE_INSTALL_FAILED, err.Error())
		return err
	}

	command := exec.Command(applier,
		Subcommand,
		StageFlag+filepath.Dir(pending.File),
		WaitFlag+fmt.Sprint(os.Getpid()))
	detach(command)

	if err := command.Start(); err != nil {
		m.fail(screensharev1.TextCode_TEXT_CODE_UPDATE_INSTALL_FAILED, err.Error())
		return err
	}

	// Released rather than waited on: it goes on running after this process is gone,
	// which is the whole reason it is a process of its own.
	if err := command.Process.Release(); err != nil {
		logger.Warnf("releasing the update applier: %v", err)
	}

	logger.Infof("release %s installs once this run exits", pending.Tag)
	return nil
}

// applierCopy puts this binary beside the staged release and answers the copy's path.
//
// Windows refuses to replace a running executable,
// so an applier reading itself out of the directory it replaces cannot finish the job.
// The same copy on every platform, one path being one thing to get right.
func applierCopy(dir string) (string, error) {
	assert.Assert(dir != "", "an applier is copied into a directory")

	exe, err := os.Executable()
	if err != nil {
		return "", err
	}

	raw, err := os.ReadFile(exe)
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, "applier"+filepath.Ext(exe))
	if err := os.WriteFile(path, raw, 0o700); err != nil {
		return "", err
	}
	return path, nil
}

// --- Writing the state ---------------------------------------------------------

// settle moves to a stage that carries no progress and clears what a failure left.
func (m *Manager) settle(stage Stage) {
	m.mu.Lock()
	m.stage, m.percent, m.failure, m.detail = stage, 0, nil, ""
	state := m.stateLocked()
	m.mu.Unlock()

	m.announce(state)
}

// step moves to a stage carrying progress.
func (m *Manager) step(stage Stage, percent int) {
	assert.Assert(percent >= 0 && percent <= 100, "a download reports a fraction of itself", percent)

	m.mu.Lock()
	m.stage, m.percent, m.failure, m.detail = stage, percent, nil, ""
	state := m.stateLocked()
	m.mu.Unlock()

	m.announce(state)
}

func (m *Manager) fail(code screensharev1.TextCode, detail string) {
	m.failText(text.Of(code), detail)
}

func (m *Manager) failText(statement *screensharev1.Text, detail string) {
	assert.IsNotNil(statement, "a failure says which failure it is")

	m.mu.Lock()
	m.stage, m.percent = StageFailed, 0
	m.failure, m.detail = statement, detail
	state := m.stateLocked()
	m.mu.Unlock()

	m.announce(state)
}
