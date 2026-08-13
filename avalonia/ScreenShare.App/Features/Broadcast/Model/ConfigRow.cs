namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// One line of the active configuration, read-only while live.
/// A muted key and a brighter value, because the key is a word a person wrote and the value is what the
/// pipeline answered.
///
/// This screen never edits one.
/// The single escape hatch is the card's "Edit in setup" link, which is the whole reason broadcast does not
/// become a second editor.
/// </summary>
/// <remarks>
/// Both halves are the backend's.
/// A row is one <c>FieldGroup</c> of the form resolved against the settings the running pipeline was built
/// from: the key is that group's title and the value its own summary, which is <c>Summary.headline</c>
/// narrowed to one group.
/// Composing the value here from the fields would be this screen inventing what a group means, and would let
/// the broadcast screen and the setup step describe one configuration in two ways (<c>docs/ipc-api.md</c>,
/// "The rule").
/// </remarks>
public sealed record ConfigRow(string Key, string Value);
