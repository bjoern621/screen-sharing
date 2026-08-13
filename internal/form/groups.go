package form

// groups is the headings a shell draws, in the order it draws them.
// The fields under each one are whichever rows of fieldTable name it, in that table's order.
//
// A group is a key and nothing else.
// What the heading reads as, and the paragraph under it, are the surface's,
// looked up by that key (api/proto/screenshare/v1/form.proto): the order and the membership are
// arguments about the domain, and the wording is an argument about a screen.
//
// The order is the domain's rather than the screen's.
// A stream is named, then the picture is chosen, then how that picture is coded,
// then what rides beside it, then how it leaves this machine, then how it comes back,
// and last where the relay carrying all of it sits.
// Each step is answerable with the ones before it settled and none of the ones after it,
// which is what lets a shell walk the groups as a wizard without the backend having said so.
//
// The two ends are where the ordering is doing work.
// The name comes first because it depends on no other field and is the one setting other people
// see: a stream not yet named is not yet anything, and every greying further down is a consequence
// of a choice made after it.
// The relay comes last because it is one question - which machine carries the stream,
// and on which of its listeners - and every part of it is answered by a default that holds for the
// relay this repository ships: the address is the machine running it, and each port's relevance was
// already decided by a leg chosen further up, the publish leg picking which ingest port is read and
// the watch leg which serving one.
// A reader on those defaults therefore never has to meet the group.
//
// The address sits with the ports rather than beside the name for the same reason.
// Where the relay is and how it is reached are one decision made once against one machine,
// and splitting them put the first half in front of a reader who had no relay to name yet and the
// second half seven groups behind it.
//
// Capture precedes encode because the capture backend fixes the publish engine,
// and the engine is what decides which codecs, pixel formats and rate-control knobs the encode
// group can offer at all (docs/glossary.md, "Publish engine").
// Reversing them would put a codec dropdown in front of the choice that says which of its entries
// are real.
//
// Audio is its own group rather than two more fields under encode, because the source and the codec
// answer to different tables - the platform's and the publish leg's - and a heading is the cheapest
// way to say that the track is a second stream rather than a property of the first
// (docs/domain-model.md).
//
// The two legs are two groups for the reason settings.proto keeps two fields:
// the publish leg is chosen once for the stream this machine sends and the watch leg per viewer,
// and one heading over both would read as one decision.
//
// The grouping is stated here and not left to a shell for the reason form.proto gives:
// it follows the domain, so a shell that regrouped it would be making an argument about the model
// rather than about layout.
// One of them is applied rather than staged, which form.proto states in full.
// The line is which settings the backend reads without being handed them: the relay poll dials the
// address for as long as the process runs, and every other group here is read by an effect that
// carries its own settings - a publish is started on what StartPublish is given,
// a viewer opens on what was saved before it.
// Only the group nobody hands over has to be written as it is edited.
var groups = []group{
	{key: GroupStream},
	{key: GroupSource},
	{key: GroupQuality},
	{key: GroupAudio},
	{key: GroupTransport},
	{key: GroupWatch},
	{key: GroupRelay, applied: true},
}
