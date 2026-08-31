package main

import (
	"context"
	"fmt"
	"os"

	"bjoernblessin.de/screenshare/internal/reach"
	"bjoernblessin.de/screenshare/internal/settings"
)

// runCheck dials the relay the stored settings name, leg by leg, and prints what each answered.
//
// Stored settings rather than a flag, so a check reads what a publish would read.
//
// Status 1 where a dialled leg did not answer,
// so a script reads the outcome without parsing the table.
// An unloadable settings file answers 1 too, reason on stderr,
// and the check still dials the defaults Load handed back,
// which tells a reader more than refusing to look.
func runCheck() int {
	s, err := settings.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	host := s.Relay.Host
	if host == "" {
		host = "none named"
	}
	fmt.Printf("relay %s\n\n", host)

	results := reach.Check(context.Background(), s)
	if reportErr := reach.Report(os.Stdout, results); reportErr != nil {
		fmt.Fprintln(os.Stderr, reportErr)
		return 1
	}
	if err != nil || reach.Failed(results) {
		return 1
	}
	return 0
}
