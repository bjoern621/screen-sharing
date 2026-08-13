// Package wire shapes the domain onto the control contract and back.
//
// Every function here is pure: a domain value in - a settings.Settings, a relay.Status,
// the capability tables - and out the screenshare.v1 message that carries it, or the reverse.
// Nothing reads state, starts work or decides anything beyond the shape.
//
// Its own package because two callers need it and neither should own it: the control service turns
// its answers into messages here, and the form package carries the repaired draft on its own
// answer, so a conversion inside either would make the other depend on a service it does not use.
//
// A field's meaning never changes on the way across.
// A conversion that renamed, rescaled or defaulted a value would be a second definition of it,
// which is the drift the contract exists to end.
// Where the two sides genuinely spell one thing differently - a capability gap naming a settings
// option by its Go JSON tag, a control naming that option by its proto field name - the translation
// is stated once here and named where it happens.
package wire
