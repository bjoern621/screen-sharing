using ScreenShare.App.Backend;

namespace ScreenShare.App.Features.Viewer.Tile.Model;

/// <summary>
/// What a tile needs from whoever put it on screen: a way to open the frames, and somewhere to say what it is
/// drawing.
///
/// It exists so the control that imports GPU handles holds no backend and no state of its own.
/// Opening a subscription is the view model's, because it is the side that knows which stream this tile
/// stands for and whether a decode has been asked for yet; drawing the frames is the control's, because it is
/// the side that can reach the compositor.
/// Neither half knows the other's business, which is what keeps the render-function discipline intact with a
/// control that is necessarily stateful (<c>avalonia/README.md</c>).
/// </summary>
public interface IFrameSource
{
    /// <summary>
    /// Subscribes to the frames of the decode this tile draws.
    /// It is called once per attach and again after a reconnect, so it opens a fresh subscription each time
    /// rather than handing out one it is holding.
    /// </summary>
    Task<FrameChannel> OpenAsync(CancellationToken cancellation);

    /// <summary>
    /// What the tile is drawing, as the tile alone can see it.
    /// It arrives on the UI loop, and a source renders it on its next pass rather than writing it onto a
    /// widget.
    /// </summary>
    void Report(TileReport report);
}

/// <summary>
/// One tile's own figures: what the backend is handing it and how much of that it drew.
///
/// They are the tile's rather than the decode's, and the difference is the whole reason they exist beside
/// <c>ReceiveState</c>.
/// The backend reports what the pipeline negotiated and what it decoded; this reports what one window got and
/// drew, which is the only place a window that is too slow becomes visible.
/// </summary>
/// <param name="Width">The pixel size the frames arrive at, which is the scaler's output and not the size the
/// tile asked for.
/// Zero before the first pool.</param> <param name="Height">The second half of that size.</param>
/// <param name="Frames">Frames handed to this tile since the subscription opened.</param>
/// <param name="Dropped">Frames the backend discarded because this tile was holding every slot.
/// It is the evidence that this window is the slow half.</param> <param name="Notice">Why the tile is not
/// drawing, empty while it is.
/// It is the backend's own sentence where there is one, because the reasons a tile cannot draw - a chain in
/// the wrong memory, a decode that ended - are facts about the pipeline.</param>
public readonly record struct TileReport(
    int Width,
    int Height,
    ulong Frames,
    ulong Dropped,
    string Notice)
{
    /// <summary>
    /// What a tile has to report before it has drawn anything.
    ///
    /// It exists because <c>default</c> is not it: a record struct's default leaves <see cref="Notice"/>
    /// null, and a tile is rendered once when it is added and before its first report, so the field every
    /// tile starts from has to be a report and not a zeroed struct.
    /// Nothing has happened yet, and the empty notice is that said out loud.
    /// </summary>
    public static readonly TileReport Nothing = new(0, 0, 0, 0, "");

    /// <summary>Whether a frame has ever been drawn, which is what separates a live tile from a connecting one.</summary>
    public bool Live => Frames > 0;
}
