using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Fields.ViewModel;

/// <summary>
/// One entry of a select or radio field, as the form described it and the screen draws it.
///
/// Every string here is written on this side: the name and the paragraph are looked up by field and
/// <see cref="Value"/>, the note and the refusal are composed from the codes the backend sent.
/// What is not decided here is whether the entry exists and whether it can be picked: <see cref="IsEnabled"/>
/// and the code behind <see cref="Reason"/> arrive already decided (docs/ipc-api.md, "The rule").
///
/// A record, so a render pass over an unchanged field produces rows that compare equal and the bound collection
/// is left alone.
/// <see cref="Choose"/> holds the field's own command instance for this value, which is what makes two passes
/// equal rather than merely alike.
/// </summary>
public sealed record OptionViewModel
{
    /// <summary>What the settings carry once this entry is picked. The identifier the backend sent, as <c>hevc_nvenc</c>.</summary>
    public required string Value { get; init; }

    public required string Label { get; init; }

    /// <summary>The trailing annotation naming what the entry was derived from, empty where there is none.</summary>
    public required string Note { get; init; }

    /// <summary>The paragraph a radio card shows under its title, empty on a select.</summary>
    public required string Detail { get; init; }

    public required bool IsSelected { get; init; }

    /// <summary>False for an entry this combination rules out. It keeps its place in the list, greyed.</summary>
    public required bool IsEnabled { get; init; }

    /// <summary>Names the limit and which side has it, so a greyed entry says what to change. Empty while the entry is live.</summary>
    public required string Reason { get; init; }

    /// <summary>Whether the backend marked the entry worth emphasising for this combination.</summary>
    public required bool IsRecommended { get; init; }

    public required DelegateCommand Choose { get; init; }

    /// <summary>
    /// Both lines under the label are drawn where both are present, the refusal first.
    /// A refused entry drawn without its paragraph states a limit and drops what the entry is, leaving the
    /// reader nothing to weigh the limit against.
    /// </summary>
    public bool HasReason => Reason.Length > 0;

    public bool HasDetail => Detail.Length > 0;

    /// <summary>
    /// The refusal as a tooltip carries it, null while the entry is live.
    /// Null rather than empty is the difference Avalonia reads: a tip of the empty string still opens, so an
    /// entry nothing is wrong with would sprout an empty box under the pointer.
    /// </summary>
    public string? Refusal => Reason.Length > 0 ? Reason : null;

    public bool HasNote => Note.Length > 0;
}
