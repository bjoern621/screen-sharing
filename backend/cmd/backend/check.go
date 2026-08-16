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
// The stored settings and not a flag: what is asked is whether this machine's relay is answering
// where this machine addresses it, so a check reads what a publish would read.
//
// The status is 1 where a leg that was dialled did not answer, so a script reads the outcome
// without parsing the table.
// A settings file that will not load answers 1 too, with its reason on standard error, and the
// check runs against what Load handed back regardless: those are the defaults, and dialling them
// tells a reader more than refusing to look.
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
