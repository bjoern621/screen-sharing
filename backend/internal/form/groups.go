package form

// groups is every heading a shell draws, in draw order.
// The fields under a heading are the rows of fieldTable naming it, in that table's order.
//
// A group carries a key and nothing else.
// The heading and the paragraph under it are the surface's, looked up by that key
// (api/proto/screenshare/v1/form.proto): the order and the membership are arguments
// about the domain, and the wording is an argument about a screen.
// A shell that regrouped or reordered them would be arguing about the model rather than the layout.
//
// The order follows the domain and not the screen.
// The picture is chosen, then how that picture is coded,
// then what rides beside it, then how it leaves this machine, then how it comes back,
// and last where the relay carrying all of it sits.
// Each step is answerable with the ones before it settled and none of the ones after it,
// so a shell walks the groups as a wizard without the backend having said so.
//
// The capture leads because it depends on no other field,
// and every greying further down follows a choice made after it.
// The relay trails because it is one question, which machine carries the stream
// and on which of its listeners, answered throughout by defaults that hold for the relay
// this repository ships, so a reader on those defaults never has to meet the group.
// The address sits with the ports rather than beside the name:
// where the relay is and how it is reached are one decision made once against one machine.
//
// Capture precedes encode because the capture backend fixes the publish engine,
// and the engine decides which codecs, pixel formats and rate-control knobs the encode group
// can offer at all (docs/glossary.md, "Publish engine").
// Audio is its own group rather than two more fields under encode because the source
// and the codec answer to different tables, the platform's and the publish leg's,
// and the track is a second stream rather than a property of the first (docs/domain-model.md).
// The two legs are two groups for the reason settings.proto keeps two fields:
// the publish leg is chosen once for the stream this machine sends and the watch leg per viewer.
//
// The app group trails all of them and is part of no walk.
// Nothing under it describes a stream, so it is answerable with none of the questions above settled
// and it is drawn where a surface asks about the app rather than about what this machine sends
// (docs/settings-editing.md).
//
// A group the backend reads without being handed it is applied rather than staged,
// which form.proto states in full.
// The relay poll dials the address for as long as the process runs,
// and the app group is read by the start after the one that wrote it.
// Every other group here is read by an effect carrying its own settings,
// a publish on what StartPublish is given and a viewer on what was saved before it.
// Such a group is written as it is edited, or a corrected address reaches the backend
// only through a publish that is refused for not reaching the relay it would replace.
var groups = []group{
	{key: GroupSource},
	{key: GroupQuality},
	{key: GroupAudio},
	{key: GroupTransport},
	{key: GroupWatch},
	{key: GroupRelay, applied: true},
	{key: GroupApp, applied: true},
}
