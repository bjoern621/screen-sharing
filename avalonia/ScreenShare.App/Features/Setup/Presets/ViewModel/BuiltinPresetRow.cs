using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.Presets.ViewModel;

/// <summary>
/// One built-in preset as the card draws it: the promise, whether the draft delivers it, and whether this
/// machine reaches it.
///
/// The other kind of preset entirely (<see cref="PresetRow"/>).
/// A saved preset is a snapshot of every field, selected while the draft equals it.
/// A built-in one is a promise about the picture, and which encoder, pixel format and capture backend deliver
/// it here is the backend's search rather than anything stored (<c>docs/presets.md</c>).
///
/// A record whose command is the owner's own instance, so two passes over one form produce rows that compare
/// equal and the list is left alone (<c>Mvvm/Reconcile.cs</c>).
/// </summary>
public sealed record BuiltinPresetRow
{
    /// <summary>The identifier the backend names this preset by, and this row's identity.</summary>
    public required string Key { get; init; }

    /// <summary>Written here rather than sent (<c>Copy/Words.cs</c>).</summary>
    public required string Name { get; init; }

    /// <summary>What it delivers, in one line (<c>Copy/Descriptions.cs</c>).</summary>
    public required string Promise { get; init; }

    /// <summary>
    /// Whether the draft already delivers the promise, as the backend derives it on every resolve.
    /// Unlike a saved preset, a field the promise says nothing about can move without unmarking the row.
    /// </summary>
    public required bool IsCurrent { get; init; }

    /// <summary>
    /// Whether this machine has a configuration delivering the promise.
    /// A false row keeps its place and its reason rather than disappearing, the treatment every other
    /// ruled-out choice gets (<c>docs/field-availability.md</c>).
    /// </summary>
    public required bool IsReachable { get; init; }

    /// <summary>Why nothing here reaches it, empty on a reachable row.</summary>
    public required string Reason { get; init; }

    /// <summary>
    /// Writes the settings the backend resolved for this preset into the draft.
    /// Nothing running changes, and a row nothing here reaches does nothing.
    /// </summary>
    public required DelegateCommand Apply { get; init; }
}
