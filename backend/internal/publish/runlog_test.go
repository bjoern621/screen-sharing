package publish

import (
	"bytes"
	"strings"
	"testing"
)

// hide stands in for the run's redactor.
func hide(text string) string { return strings.ReplaceAll(text, "s3cret", "<redacted>") }

// A child writes when it writes, so a secret lands split across two of them.
// What holds no newline yet is kept, and the secret is whole by the time it is looked for.
func TestASecretSplitAcrossWritesIsStillHidden(t *testing.T) {
	var held bytes.Buffer
	w := hiding(&held, hide)

	w.Write([]byte("no element \"s3"))
	w.Write([]byte("cret\"\n"))

	if got := held.String(); strings.Contains(got, "s3cret") {
		t.Fatalf("the log carries %q", got)
	}
	if got := held.String(); !strings.Contains(got, "<redacted>") {
		t.Fatalf("the log carries %q, and names no redaction", got)
	}
}

// A child that died mid-line said what it said, and the tail is what a reader is shown.
func TestALastLineWithNoNewlineIsPassedOnHidden(t *testing.T) {
	var held bytes.Buffer
	w := hiding(&held, hide)

	w.Write([]byte("passphrase=s3cret"))
	if held.String() != "" {
		t.Fatalf("an unfinished line was passed on as %q", held.String())
	}

	if err := w.Flush(); err != nil {
		t.Fatalf("flushing: %v", err)
	}
	if got := held.String(); got != "passphrase=<redacted>" {
		t.Fatalf("the flush wrote %q", got)
	}
}

// A copier reads a short write as a failure to write, so every write answers the whole of what it took.
func TestEveryWriteAnswersWhatItTook(t *testing.T) {
	w := hiding(&bytes.Buffer{}, hide)

	for _, chunk := range []string{"held\n", "no newline yet", " and now\n"} {
		n, err := w.Write([]byte(chunk))
		if err != nil || n != len(chunk) {
			t.Fatalf("writing %q answered %d, %v", chunk, n, err)
		}
	}
}

// A run carrying no secret takes no redactor, and what it writes passes through.
func TestNoRedactorPassesEverythingThrough(t *testing.T) {
	var held bytes.Buffer
	w := hiding(&held, nil)

	w.Write([]byte("passphrase=s3cret"))
	if got := held.String(); got != "passphrase=s3cret" {
		t.Fatalf("the log carries %q, want what was written", got)
	}
}

// The wiring is the contract: a supervised child's secrets cross in the environment,
// so arguments still carrying one are a bug this side, caught before the child is started.
func TestASupervisedChildIsRefusedArgumentsCarryingASecret(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a child was spawned with a secret in its arguments")
		}
	}()

	supervise(superviseConfig{
		exe:    "true",
		tag:    "secret-in-argv-probe",
		args:   []string{"passphrase=s3cret"},
		redact: hide,
	})
}
