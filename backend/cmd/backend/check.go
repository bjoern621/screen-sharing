package main

import (
	"context"
	"fmt"
	"os"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/groupclient"
	"bjoernblessin.de/screenshare/internal/member"
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

	results := reach.Check(context.Background(), withRelayToken(s))
	if reportErr := reach.Report(os.Stdout, results); reportErr != nil {
		fmt.Fprintln(os.Stderr, reportErr)
		return 1
	}
	if err != nil || reach.Failed(results) {
		return 1
	}
	return 0
}

// withRelayToken is s carrying the credential the HTTP legs are dialled with, and s unchanged
// where none can be traded for.
//
// The trade a publish makes (internal/app, settingsForCommand), made here for the same reason:
// the relay refuses a reader holding no token, so a check dialled without one reads 401 on every
// HTTP leg whatever those listeners are doing.
//
// Discord mode brokers its token through the running app, so a check from a terminal dials those
// legs without one, and every failure here does the same: the group service holds a row of its own
// to fail in.
func withRelayToken(s settings.Settings) settings.Settings {
	base, ok := s.Relay.GroupService()
	if !ok || s.Relay.DiscordMode || s.Relay.GroupKey == "" {
		return s
	}

	groupKey, err := group.ParseKey(s.Relay.GroupKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "the stored group key does not parse, so the HTTP legs are dialled without a token: %v\n", err)
		return s
	}
	identity, _, err := member.Load(groupKey.ID())
	if err != nil {
		fmt.Fprintf(os.Stderr, "the member identity is not readable, so a token names no member: %v\n", err)
	}

	token, err := groupclient.New().Token(base, s.Relay.GroupKey, identity.Secret)
	if err != nil {
		fmt.Fprintf(os.Stderr, "no relay token for this group, so the HTTP legs are dialled without one: %v\n", err)
		return s
	}
	s.Relay.Token = token
	return s
}
