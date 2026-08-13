using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.Presets.ViewModel;

/// <summary>
/// One saved preset as the card draws it: its name, whether the draft is it, and what can be done to it.
///
/// A record whose commands are the owner's own instances, so two passes over an unchanged store produce rows
/// that compare equal and the list is left alone (<c>Mvvm/Reconcile.cs</c>).
/// </summary>
public sealed record PresetRow
{
    /// <summary>Also the identity it is replaced and deleted by.</summary>
    public required string Name { get; init; }

    /// <summary>
    /// Whether the draft equals this preset, field for field.
    ///
    /// Derived on every pass rather than remembered from the press that applied it.
    /// A snapshot claims nothing about a region of the settings, so selected can only mean equal: an edit
    /// moving one field unmarks the row, and no stored selection is left to disagree with the settings
    /// (<c>docs/presets.md</c>, "Saved presets").
    /// </summary>
    public required bool IsCurrent { get; init; }

    /// <summary>
    /// Writes this preset into the draft, whole.
    /// Nothing running changes.
    /// </summary>
    public required DelegateCommand Apply { get; init; }

    /// <summary>Takes it out of the store, which crosses to the backend.</summary>
    public required PendingCommand Delete { get; init; }
}
