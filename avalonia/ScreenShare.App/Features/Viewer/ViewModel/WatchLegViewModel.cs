using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.ViewModel;

/// <summary>
/// One transport a stream can be watched over, as a row of the player menu.
///
/// The value is an option of the form's watch-leg field, so a protocol added to the transport registry
/// arrives here with nothing in this module to edit; the word on the row is this side's.
///
/// <b>A leg is offered whether or not this stream's format can travel on it.</b> That verdict is answered
/// against the stream as the viewer is opened, and the relay's snapshot can be older than the stream, so
/// greying from a stale format would refuse a viewer that would have worked.
/// The backend refuses naming the format, and the screen shows that sentence.
///
/// <b>The option's own verdict is a different fact, and this row draws it.</b> Whether a player on this
/// machine can be opened on the protocol at all is the availability pass's answer, a property of the
/// receiver rather than of a stream, and it does not go stale between a snapshot and a press.
/// An entry it ruled out greys and carries its reason, as every other option in the product does
/// (<c>docs/field-availability.md</c>).
///
/// The reason sits in the row rather than hanging off it as a tip.
/// A disabled control in Avalonia takes no pointer events, so a tip on a greyed row is a sentence nobody can
/// open.
///
/// A record, so a pass over an unchanged row produces legs that compare equal.
/// The command is made once per leg by the row that owns it.
/// </summary>
public sealed record WatchLegViewModel
{
    /// <summary>Transport as the contract names it, which is what <c>StartWatch</c> takes.</summary>
    public required string Value { get; init; }

    public required string Label { get; init; }

    /// <summary>Whether a viewer of this machine's is open on this stream over this leg.</summary>
    public required bool IsOpen { get; init; }

    /// <summary>As the availability pass answered, except that an open leg stays pressable so the press can close it.</summary>
    public required bool IsEnabled { get; init; }

    /// <summary>Why not, empty where it can. This side's sentence for the backend's code.</summary>
    public required string Reason { get; init; }

    public bool HasReason => Reason.Length > 0;

    /// <summary>
    /// Opens a viewer on this leg, or closes the open one.
    /// Holds whether its own call is still out, which is what the control waits on.
    /// </summary>
    public required PendingCommand Toggle { get; init; }
}
