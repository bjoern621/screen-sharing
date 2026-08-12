using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.Presets.ViewModel;

/// <summary>
/// One built-in preset, as the card draws it: what it promises, whether the draft delivers it,
/// and whether this machine has a way of reaching it.
///
/// It is the other kind of preset entirely (<see cref="PresetRow"/>). A saved preset is a
/// snapshot of every field and is selected while the draft equals it; a built-in one is a
/// promise about the picture, and which encoder, pixel format and capture backend deliver it
/// here is the backend's search rather than anything stored (<c>docs/presets.md</c>).
///
/// A record whose command is the owner's own instance, so two passes over one form produce rows
/// that compare equal and the list is left alone (<c>Mvvm/Reconcile.cs</c>).
/// </summary>
public sealed record BuiltinPresetRow
{
    /// <summary>The identifier the backend names this preset by, and this row's identity.</summary>
    public required string Key { get; init; }

    /// <summary>What it is called (<c>Copy/Words.cs</c>).</summary>
    public required string Name { get; init; }

    /// <summary>What it delivers, in one line (<c>Copy/Descriptions.cs</c>).</summary>
    public required string Promise { get; init; }

    /// <summary>
    /// Whether the draft already delivers this preset's promise.
    ///
    /// The backend derives it from the settings on every resolve, so an edit that stays inside
    /// the promise keeps the dot and one that leaves it puts the dot out - and unlike a saved
    /// preset, a field the promise says nothing about can move without unmarking the row.
    /// </summary>
    public required bool IsCurrent { get; init; }

    /// <summary>
    /// Whether this machine has a configuration that delivers the promise. False rows keep their
    /// place and their reason rather than disappearing, which is the treatment every other
    /// ruled-out choice gets (<c>docs/field-availability.md</c>).
    /// </summary>
    public required bool IsReachable { get; init; }

    /// <summary>Why nothing here reaches it, empty on a row that is reachable.</summary>
    public required string Reason { get; init; }

    /// <summary>
    /// Writes the settings the backend resolved for this preset into the draft. It changes
    /// nothing that is running, and it does nothing on a row nothing here reaches.
    /// </summary>
    public required DelegateCommand Apply { get; init; }
}
