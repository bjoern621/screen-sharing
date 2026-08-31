using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.ViewModel;

/// <summary>
/// One transport a stream can be watched over, as a row of the player menu.
///
/// Value is an entry of the catalog's player roster,
/// so a protocol added to the transport registry needs no edit here.
/// Word on the row is this side's.
///
/// <b>Every row is pressable.</b>
/// The roster names the legs a player on this machine opens, so no entry is one it cannot.
/// Whether this stream's format travels on the leg is answered against the stream as the viewer opens:
/// the relay's snapshot can be older than the stream,
/// so greying from a stale format would refuse a viewer that would have worked.
/// The backend refuses naming the format, and the screen shows that sentence.
///
/// A record, so a pass over an unchanged row produces legs that compare equal.
/// The command is made once per leg by the row that owns it.
/// </summary>
public sealed record WatchLegViewModel
{
    /// <summary>Transport as the contract names it, what <c>StartWatch</c> takes.</summary>
    public required string Value { get; init; }

    public required string Label { get; init; }

    /// <summary>Whether a viewer of this machine's is open on this stream over this leg.</summary>
    public required bool IsOpen { get; init; }

    /// <summary>Opens a viewer on this leg, or closes the open one.</summary>
    public required PendingCommand Toggle { get; init; }
}
