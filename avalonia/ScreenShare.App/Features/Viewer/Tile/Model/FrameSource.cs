using ScreenShare.App.Backend;

namespace ScreenShare.App.Features.Viewer.Tile.Model;

/// <summary>
/// What a tile needs from whoever placed it: frames to open, and somewhere to report what it draws.
///
/// Exists so the control importing GPU handles holds no backend and no state of its own.
/// Opening a subscription is the view model's, knowing which stream the tile stands for and whether a decode has
/// been asked for, and drawing is the control's, being the side that reaches the compositor.
/// The split keeps the render-function discipline with a control that is necessarily stateful
/// (<c>avalonia/README.md</c>).
/// </summary>
public interface IFrameSource
{
    /// <summary>
    /// Opens a subscription to the frames this tile draws.
    /// Called once per attach and again after a reconnect, so each call opens a fresh subscription rather than
    /// handing out one it holds.
    /// </summary>
    Task<FrameChannel> OpenAsync(CancellationToken cancellation);

    /// <summary>
    /// What the tile is drawing, as only the tile can see it.
    /// Arrives on the UI loop, and a source renders it on its next pass rather than writing it onto a widget.
    /// </summary>
    void Report(TileReport report);
}

/// <summary>
/// One tile's own figures: what the backend handed it and how much of that it drew.
/// The tile's and not the decode's, hence their sitting beside <c>ReceiveState</c>.
/// A window too slow to take a frame is invisible from the backend, which sees a slot of its pool that has not
/// come back.
/// </summary>
/// <param name="Width">
/// Pixel width the frames arrive at, the scaler's output and not the size this tile requested.
/// Zero before the first pool.
/// </param>
/// <param name="Height">Pixel height, on the same terms.</param>
/// <param name="Frames">Frames handed to this tile since the subscription opened, a total and not a rate.</param>
/// <param name="Dropped">
/// Frames the backend discarded for this tile holding every slot, counted from the same point.
/// Evidence that this window is the slow half.
/// </param>
/// <param name="Notice">
/// Why the tile is not drawing, empty while it is.
/// The backend's own sentence where there is one, a chain in the wrong memory or a decode that ended being facts
/// about the pipeline.
/// </param>
public readonly record struct TileReport(
    int Width,
    int Height,
    ulong Frames,
    ulong Dropped,
    string Notice)
{
    /// <summary>
    /// What a tile reports before it has drawn anything.
    /// <c>default</c> is not it: a record struct's default leaves <see cref="Notice"/> null, and a tile is
    /// rendered once when it is added, before its first report.
    /// </summary>
    public static readonly TileReport Nothing = new(0, 0, 0, 0, "");

    /// <summary>A frame has been drawn, which separates a live tile from a connecting one.</summary>
    public bool Live => Frames > 0;
}
