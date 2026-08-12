using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.Presets.ViewModel;

/// <summary>
/// One saved preset, as the card draws it: what it is called, whether the draft is it, and the
/// two things that can be done to it.
///
/// A record whose commands are the owner's own instances, so two passes over an unchanged store
/// produce rows that compare equal and the list is left alone (<c>Mvvm/Reconcile.cs</c>).
/// </summary>
public sealed record PresetRow
{
    /// <summary>The name it was saved under, which is also the identity it is replaced and deleted by.</summary>
    public required string Name { get; init; }

    /// <summary>
    /// Whether the draft is this preset, field for field.
    ///
    /// Derived on every pass rather than remembered from the press that applied it. A snapshot
    /// carries no claim about a region of the settings, so being selected can only mean being
    /// equal: an edit that moves one field off it unmarks the row, and there is no stored
    /// selection left to disagree with the settings (<c>docs/presets.md</c>, "Saved presets").
    /// </summary>
    public required bool IsCurrent { get; init; }

    /// <summary>Writes this preset into the draft, whole. It changes nothing that is running.</summary>
    public required DelegateCommand Apply { get; init; }

    /// <summary>Takes it out of the store, which crosses to the backend.</summary>
    public required PendingCommand Delete { get; init; }
}
