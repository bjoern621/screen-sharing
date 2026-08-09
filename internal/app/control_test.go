package app

import (
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The publish state has two readers that hold each other to one rule: the frontend
// renders "attempt n of m" off the attempt and the budget, and the contract asserts
// that a state nothing is retrying carries neither figure (wire.PublishState).
//
// These tests run the real producer through the real conversion into the contract, so
// a state that is legal on one surface and a panic on the other fails here rather than
// on the first shell that connects while a stream is healthy.

// liveHandle is a pipeline that is running. Nothing here stops it, because what these
// tests read is the state around it.
type liveHandle struct{}

func (liveHandle) Running() bool { return true }
func (liveHandle) Stop()         {}

// TestALiveStreamCarriesNoRetryFigures: a stream that is carrying frames is spending
// no attempts, so the pair that counts them stays at zero. A budget reported beside it
// would name attempts nothing is spending, and the contract refuses to carry one.
func TestALiveStreamCarriesNoRetryFigures(t *testing.T) {
	a := &App{run: &publishRun{settings: settings.Settings{
		Publish: settings.Publish{
			Name: "bob",
		},
	}, handle: liveHandle{}}}

	state := a.GetPublishState()
	if !state.Publishing || state.Retrying {
		t.Fatalf("a running pipeline reports publishing=%v retrying=%v, want true and false", state.Publishing, state.Retrying)
	}
	if state.Attempt != 0 || state.Budget != 0 {
		t.Errorf("a live stream reports attempt %d of %d, want neither figure", state.Attempt, state.Budget)
	}

	// The contract asserts the rule as well, so a state that broke it panics here
	// instead of on the shell that read it.
	wire.PublishState(publishSnapshot(state))
}

// TestAPendingRetryCarriesTheAttemptAndTheBudget: between attempts the two figures are
// the whole of what the state says, and they reach the contract unchanged.
func TestAPendingRetryCarriesTheAttemptAndTheBudget(t *testing.T) {
	a := &App{retry: &publishRetry{settings: settings.Settings{
		Publish: settings.Publish{
			Name: "bob",
		},
	}, attempts: 2}}

	state := a.GetPublishState()
	if !state.Publishing || !state.Retrying {
		t.Fatalf("a pending relaunch reports publishing=%v retrying=%v, want both true", state.Publishing, state.Retrying)
	}
	if state.Attempt != 2 || state.Budget != len(publishBackoff) {
		t.Errorf("a pending relaunch reports attempt %d of %d, want 2 of %d", state.Attempt, state.Budget, len(publishBackoff))
	}

	snapshot := publishSnapshot(state)
	retry := snapshot.Retry()
	if retry == nil {
		t.Fatalf("the contract's snapshot carries no retry, want attempt %d of %d", state.Attempt, state.Budget)
	}
	if retry.Attempt != state.Attempt || retry.Budget != state.Budget {
		t.Errorf("the contract's snapshot carries attempt %d of %d, want %d of %d",
			retry.Attempt, retry.Budget, state.Attempt, state.Budget)
	}
	wire.PublishState(snapshot)
}

// TestAStoppedStreamCarriesNothing: with no pipeline and no relaunch pending there is
// no stream to describe, and the settings field stays absent rather than crossing as a
// stream configured entirely wrong.
func TestAStoppedStreamCarriesNothing(t *testing.T) {
	a := &App{}

	state := a.GetPublishState()
	if state.Publishing || state.Retrying || state.Settings != nil {
		t.Fatalf("a stopped app reports publishing=%v retrying=%v settings=%v, want false, false and none",
			state.Publishing, state.Retrying, state.Settings)
	}

	snapshot := publishSnapshot(state)
	if snapshot.Live != nil {
		t.Errorf("a stopped app carries a live stream, want none")
	}
	if snapshot.Retry() != nil {
		t.Errorf("a stopped app carries a retry, want none")
	}
	wire.PublishState(snapshot)
}

// TestStoppingTheControlServiceTwiceIsOneStop: the shutdown path can be reached more
// than once, and the second run must find nothing left to stop rather than a second
// GracefulStop on a server that has already released its socket. The flag it sets is
// the other half of the same contract: it is what a service still opening its socket
// is stopped by when the shutdown got there first.
func TestStoppingTheControlServiceTwiceIsOneStop(t *testing.T) {
	a := &App{}

	a.stopControl()
	a.stopControl()

	if !a.controlStopped {
		t.Error("a stopped app does not report the control service stopped, want a socket opening after it to be closed again")
	}
	if a.control != nil {
		t.Error("a stopped app still holds a control service, want the handle released")
	}
}
