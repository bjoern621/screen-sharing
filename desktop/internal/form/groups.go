package form

// groups is the headings a shell draws, in the order it draws them. The fields under
// each one are whichever rows of fieldTable name it, in that table's order.
//
// A group is a key and nothing else. What the heading reads as, and the paragraph under
// it, are the surface's, looked up by that key
// (api/proto/screenshare/v1/form.proto): the order and the membership are arguments
// about the domain, and the wording is an argument about a screen.
//
// The order is the domain's rather than the screen's. A stream is named and pointed at
// a relay, then the picture is chosen, then how that picture is coded, then what rides
// beside it, then how it leaves this machine, and last how it comes back. Each step is
// answerable with the ones before it settled and none of the ones after it, which is
// what lets a shell walk the groups as a wizard without the backend having said so.
//
// The two ends are where the ordering is doing work. The name and the relay come first
// because they are the only fields that depend on no other: a stream not yet named is
// not yet anything, and every greying further down is a consequence of a choice made
// after these two. The relay's listener ports come last because each of them is a
// number some earlier choice already decided the relevance of - the publish leg picks
// which ingest port is read, the watch leg which serving one - so a reader who leaves
// the defaults alone never has to meet them.
//
// Capture precedes encode because the capture backend fixes the publish engine, and the
// engine is what decides which codecs, pixel formats and rate-control knobs the encode
// group can offer at all (docs/glossary.md, "Publish engine"). Reversing them would put
// a codec dropdown in front of the choice that says which of its entries are real.
//
// Audio is its own group rather than two more fields under encode, because the source
// and the codec answer to different tables - the platform's and the publish leg's - and
// a heading is the cheapest way to say that the track is a second stream rather than a
// property of the first (docs/domain-model.md).
//
// The two legs are two groups for the reason settings.proto keeps two fields: the
// publish leg is chosen once for the stream this machine sends and the watch leg per
// viewer, and one heading over both would read as one decision.
//
// The grouping is stated here and not left to a shell for the reason form.proto gives:
// it follows the domain, so a shell that regrouped it would be making an argument about
// the model rather than about layout.
var groups = []group{
	{key: GroupStream},
	{key: GroupSource},
	{key: GroupQuality},
	{key: GroupAudio},
	{key: GroupTransport},
	{key: GroupWatch},
	{key: GroupAdvanced},
}
