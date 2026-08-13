// Package wire shapes the domain onto the control contract and back.
//
// Every function here is pure: it takes domain values - a settings.Settings, a relay.Status,
// the capability tables - and returns the screenshare.v1 message that carries them,
// or takes the message and returns the domain value.
// It reads no state, starts nothing and decides nothing beyond the shape.
//
// It exists as its own package because two callers need it and neither should own it.
// The control service turns its answers into messages here, and the form package carries the
// repaired draft on its answer, so a conversion living inside either would make the other depend on
// a service it does not use.
//
// The one rule worth stating: a field's meaning never changes on the way across.
// A conversion that renamed, rescaled or defaulted a value would be a second definition of that
// value, which is the drift the contract exists to end.
// Where the two sides genuinely spell something differently - a capability gap naming a settings
// option by its Go JSON tag, a control naming the same option by its proto field name - the
// translation is stated once here and named where it happens.
package wire
