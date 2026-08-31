package app

import (
	"errors"
	"testing"
	"time"
)

// exit stands in for what a dead child leaves behind.
// Its text reaches no decision, only its presence does.
var exit = errors.New("publish exited: signal: aborted (core dumped)")

// A pipeline that ended without failing was ended by something that wanted it ended,
// and a relaunch would undo that.
func TestACleanExitIsNotRetried(t *testing.T) {
	if _, _, retry := publishRetryAfter(nil, time.Hour, 0); retry {
		t.Error("a clean exit scheduled a relaunch, want none")
	}
}

// Settings this machine cannot run fail the same way every attempt, so the budget has to bind.
// The AV1 encoder that hangs the GPU is the case: every attempt costs a driver reset,
// and nothing about the next one differs from the last.
func TestAPipelineFailingAtLaunchWalksTheBackoffOnce(t *testing.T) {
	for attempts := range len(publishBackoff) {
		spent, wait, retry := publishRetryAfter(exit, 4*time.Second, attempts)
		if !retry {
			t.Fatalf("attempt %d was not retried, want the budget to still hold", attempts)
		}
		if spent != attempts {
			t.Errorf("spent after attempt %d = %d, want the attempts it inherited", attempts, spent)
		}
		if wait != publishBackoff[attempts] {
			t.Errorf("wait after attempt %d = %s, want %s", attempts, wait, publishBackoff[attempts])
		}
	}

	spent, _, retry := publishRetryAfter(exit, 4*time.Second, len(publishBackoff))
	if retry {
		t.Error("the attempt past the budget was retried, want the budget to bind")
	}
	if spent != len(publishBackoff) {
		t.Errorf("spent at the end = %d, want the full budget, which is what the message reports", spent)
	}
}

// A publish pipeline dying by itself usually means a relay restart or a source going away,
// which take seconds.
// A flat retry would spend the whole budget inside the outage it is waiting out.
func TestTheBackoffGrows(t *testing.T) {
	for i := 1; i < len(publishBackoff); i++ {
		if publishBackoff[i] <= publishBackoff[i-1] {
			t.Errorf("backoff[%d] = %s, want more than the %s before it", i, publishBackoff[i], publishBackoff[i-1])
		}
	}
}

// A pipeline that carried the stream proved the settings run on this machine,
// so what an earlier outage cost says nothing about this one,
// and the next failure starts at the first delay.
func TestAHealthyRunRefillsTheBudget(t *testing.T) {
	spent, wait, retry := publishRetryAfter(exit, publishHealthy, len(publishBackoff))
	if !retry {
		t.Fatal("a healthy run was not retried, want a full budget after it")
	}
	if spent != 0 {
		t.Errorf("spent after a healthy run = %d, want 0", spent)
	}
	if wait != publishBackoff[0] {
		t.Errorf("wait after a healthy run = %s, want the first delay %s", wait, publishBackoff[0])
	}
}

// A relay that is not up and an encoder that hangs the GPU exit under the same status and signal,
// so how far the pipeline got is the only thing telling them apart.
// The bound is what the decision turns on, and both sides of it are pinned here.
func TestTheHealthyBoundIsWhatSeparatesTheTwo(t *testing.T) {
	spent, _, _ := publishRetryAfter(exit, publishHealthy-time.Millisecond, 1)
	if spent != 1 {
		t.Errorf("spent just under the bound = %d, want the attempt to be charged", spent)
	}

	spent, _, _ = publishRetryAfter(exit, publishHealthy, 1)
	if spent != 0 {
		t.Errorf("spent at the bound = %d, want the budget refilled", spent)
	}
}
