using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.ViewModel;

/// <summary>
/// One leg the relay serves a player page for, on one row of the browser menu.
///
/// <b>An action and not a state, which is the whole difference from
/// <see cref="WatchLegViewModel"/>.</b> A page opens in a browser this app does not own: it
/// cannot be read back and it cannot be closed, so there is nothing here to tick and no
/// second press that would undo the first. A row that showed a tick would be showing a
/// guess, and one that toggled would be offering a close nothing can perform.
///
/// The value and the label are the backend's for the reason a watch leg's are: the legs come
/// off the catalog, so a protocol the relay gains a page for appears here with nothing in
/// this module to edit.
///
/// A record, so a pass over an unchanged row produces legs that compare equal; the command
/// is made once per leg by the row that owns it.
/// </summary>
public sealed record BrowserLegViewModel
{
    /// <summary>The transport as the contract names it, which is what <c>OpenInBrowser</c> is given.</summary>
    public required string Value { get; init; }

    /// <summary>What the row shows, written on this side.</summary>
    public required string Label { get; init; }

    /// <summary>
    /// Opens the relay's page for this stream on this leg. It carries whether its own call is
    /// still out, which is what keeps a second press from asking again while the first is
    /// being answered.
    /// </summary>
    public required PendingCommand Open { get; init; }
}
