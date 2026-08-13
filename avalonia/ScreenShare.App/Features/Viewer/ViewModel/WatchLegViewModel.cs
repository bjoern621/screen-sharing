using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.ViewModel;

/// <summary>
/// One transport a stream can be watched over, on one row.
///
/// The value and the label are the backend's: they are an option of the form's watch-leg field, so a protocol
/// added to the transport registry appears here with nothing in this module to edit, and a shell that wrote
/// "SRT" itself would be a shell holding the vocabulary.
///
/// <b>The leg is offered whether or not this stream's format can travel on it, and that is deliberate.</b>
/// Whether a leg carries a format is the backend's answer and it is answered against the stream being opened
/// - the relay's snapshot can be older than the stream, and a shell that greyed a leg from a stale format
/// would refuse a viewer that would have worked.
/// The backend refuses with the format named, and the row shows that sentence.
///
/// <b>That is not the same fact as the option's own verdict, and this row draws both.</b> The availability
/// pass answers whether a player on this machine can be opened on the protocol at all, which is a property of
/// the receiver rather than of a stream, and nothing about it goes stale between a snapshot and a press.
/// An entry it ruled out is greyed and carries its reason, as every other option in the product is
/// (<c>docs/field-availability.md</c>).
/// This row used to throw both away and offer every leg live, so the one rule the design states about an
/// unavailable option was the rule this menu did not keep.
///
/// The reason is drawn in the row rather than hung off it as a tip.
/// A disabled control in Avalonia takes no pointer events, so a tooltip on a greyed row is a sentence nobody
/// can open - and a greyed row whose reason cannot be read is the blank the treatment exists to prevent.
///
/// A record so a pass over an unchanged row produces legs that compare equal; the command is made once per
/// leg by the row that owns it, which is what makes two passes equal rather than merely alike.
/// </summary>
public sealed record WatchLegViewModel
{
    /// <summary>The transport as the contract names it, which is what <c>StartWatch</c> is given.</summary>
    public required string Value { get; init; }

    /// <summary>What the control shows, written in Go.</summary>
    public required string Label { get; init; }

    /// <summary>Whether this machine has a viewer open on this leg for this stream.</summary>
    public required bool IsOpen { get; init; }

    /// <summary>Whether the row can be pressed, as the backend answered for this option.</summary>
    public required bool IsEnabled { get; init; }

    /// <summary>Why it cannot, and empty where it can. It is the shell's sentence for the backend's code.</summary>
    public required string Reason { get; init; }

    /// <summary>Whether there is a sentence to draw under the name.</summary>
    public bool HasReason => Reason.Length > 0;

    /// <summary>
    /// Opens a viewer on this leg, or closes the one that is open.
    /// It carries whether its own call is still out, which is what the control waits on.
    /// </summary>
    public required PendingCommand Toggle { get; init; }
}
