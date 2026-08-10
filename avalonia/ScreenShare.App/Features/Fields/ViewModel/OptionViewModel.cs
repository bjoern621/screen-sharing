using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Fields.ViewModel;

/// <summary>
/// One entry of a select or radio field, as the form described it and the screen draws it.
///
/// Every string here was written by the backend. The shell decides which of them is bold,
/// which is dimmed and where they sit; it does not decide what any of them says, and it
/// does not decide whether the entry is reachable - <see cref="IsEnabled"/> and
/// <see cref="Reason"/> arrive already decided (docs/ipc-api.md, "The rule").
///
/// A record, so a render pass over an unchanged field produces rows that compare equal and
/// the bound collection is left alone. <see cref="Choose"/> holds the field's own command
/// instance for this value, which is what makes two passes equal rather than merely alike.
/// </summary>
public sealed record OptionViewModel
{
    /// <summary>What the settings would carry if this entry were picked.</summary>
    public required string Value { get; init; }

    public required string Label { get; init; }

    /// <summary>The trailing annotation naming what the entry was derived from, empty where there is none.</summary>
    public required string Note { get; init; }

    /// <summary>The paragraph a radio card shows under its title, empty on a select.</summary>
    public required string Detail { get; init; }

    public required bool IsSelected { get; init; }

    /// <summary>False for an entry the current settings rule out. It stays in the list, greyed.</summary>
    public required bool IsEnabled { get; init; }

    /// <summary>Why the entry is out, which names the limit and which side has it. Empty while it is in.</summary>
    public required string Reason { get; init; }

    /// <summary>Whether the backend marked this entry worth emphasising for this combination.</summary>
    public required bool IsRecommended { get; init; }

    public required DelegateCommand Choose { get; init; }

    /// <summary>What the row says under its label: the reason where there is one, the detail otherwise.</summary>
    public string Body => Reason.Length > 0 ? Reason : Detail;

    /// <summary>
    /// The refusal as a tooltip carries it, and null while the entry is live.
    ///
    /// Null rather than empty, because that is the difference Avalonia reads: a tooltip
    /// whose tip is the empty string still opens, so an entry nothing is wrong with would
    /// sprout an empty box under the pointer.
    /// </summary>
    public string? Refusal => Reason.Length > 0 ? Reason : null;

    public bool HasBody => Body.Length > 0;

    public bool HasNote => Note.Length > 0;
}
