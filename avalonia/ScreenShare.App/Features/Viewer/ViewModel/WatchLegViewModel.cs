using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.ViewModel;

/// <summary>
/// One transport a stream can be watched over, on one row.
///
/// The value and the label are the backend's: they are an option of the form's watch-leg field,
/// so a protocol added to the transport registry appears here with nothing in this module to
/// edit, and a shell that wrote "SRT" itself would be a shell holding the vocabulary.
///
/// <b>The leg is offered whether or not this stream's format can travel on it, and that is
/// deliberate.</b> Whether a leg carries a format is the backend's answer and it is answered
/// against the stream being opened - the relay's snapshot can be older than the stream, and a
/// shell that greyed a leg from a stale format would refuse a viewer that would have worked.
/// The backend refuses with the format named, and the row shows that sentence.
///
/// A record so a pass over an unchanged row produces legs that compare equal; the command is
/// made once per leg by the row that owns it, which is what makes two passes equal rather than
/// merely alike.
/// </summary>
public sealed record WatchLegViewModel
{
    /// <summary>The transport as the contract names it, which is what <c>StartWatch</c> is given.</summary>
    public required string Value { get; init; }

    /// <summary>What the control shows, written in Go.</summary>
    public required string Label { get; init; }

    /// <summary>Whether this machine has a viewer open on this leg for this stream.</summary>
    public required bool IsOpen { get; init; }

    /// <summary>
    /// Opens a viewer on this leg, or closes the one that is open. It carries whether its own
    /// call is still out, which is what the control waits on.
    /// </summary>
    public required PendingCommand Toggle { get; init; }
}
