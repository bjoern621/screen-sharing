package ffmpeg

// Option adjusts what a run does beside launching the child.
//
// Variadic rather than more parameters on Start: each entry is work a caller that asked for it gets
// and none for one that did not, and a run with nothing to hide passes nothing.
type Option func(*options)

type options struct {
	redact func(string) string
}

// WithRedactor hides the run's secrets in what the log carries, and no redactor hides nothing.
//
// The first line of every run log is the child's whole command line, which spells out the relay
// token and the SRT passphrase: the app then advertises that log's path and offers to open it, so a
// reader forwarding one to get help forwards both.
// The passphrase is the one that matters, being relay-wide and expiring with nothing
// (transport.Redact builds the function).
func WithRedactor(redact func(string) string) Option {
	return func(o *options) { o.redact = redact }
}
