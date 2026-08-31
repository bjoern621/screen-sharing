using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.ViewModel;

/// <summary>
/// One leg the relay serves a player page for, as a row of the browser menu.
///
/// <b>An action, where <see cref="WatchLegViewModel"/> is a state.</b>
/// The page opens in a browser this app does not own, so nothing to read back and nothing to close.
///
/// Legs come off the catalog, so a protocol the relay gains a page for needs no edit here.
/// Word on the row is this side's.
///
/// A record, so a pass over an unchanged row produces legs that compare equal.
/// The command is made once per leg by the row that owns it.
/// </summary>
public sealed record BrowserLegViewModel
{
    /// <summary>Transport as the contract names it, what <c>OpenInBrowser</c> takes.</summary>
    public required string Value { get; init; }

    public required string Label { get; init; }

    /// <summary>Opens the relay's page for this stream over this leg.</summary>
    public required PendingCommand Open { get; init; }
}
