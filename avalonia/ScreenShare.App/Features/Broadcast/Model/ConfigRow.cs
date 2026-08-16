namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// One line of the configuration in force, read-only while live.
/// Editing goes through the card's "Edit in setup" link alone, which keeps broadcast from becoming a second
/// editor.
/// </summary>
/// <remarks>
/// Both halves come from the backend.
/// A row is one <c>FieldGroup</c> of the form resolved against the settings the running pipeline was built from:
/// the key is that group's title, the value its own summary, <c>Summary.headline</c> narrowed to one group.
/// Composing the value here out of the fields would invent what a group means, and would let this screen and
/// the setup step describe one configuration two ways (<c>docs/ipc-api.md</c>, "The rule").
/// </remarks>
public sealed record ConfigRow(string Key, string Value);
