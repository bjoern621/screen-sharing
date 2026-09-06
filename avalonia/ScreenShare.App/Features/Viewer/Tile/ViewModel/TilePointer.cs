using ScreenShare.Api.V1;
using ScreenShare.App.Backend;

namespace ScreenShare.App.Features.Viewer.Tile.ViewModel;

/// <summary>
/// The publishing machine's pointer, drawn over the picture that does not carry it.
///
/// A stream whose publisher sends the position instead of drawing it arrives with no pointer in the frames,
/// so a viewer that draws none shows a stream with no mouse in it
/// (<c>docs/capture-architecture.md</c>, "The pointer").
///
/// Followed for exactly as long as this tile draws frames.
/// The subscription is opened on the token the frame channel is opened on, so a tile that stopped drawing
/// stops asking, and a popped-out stream is followed by the window that draws it rather than by the grid slot
/// that does not.
/// </summary>
public sealed partial class TileViewModel
{
    /// <summary>Newest position, null where the stream carries none.</summary>
    private PointerPosition? _at;

    /// <summary>Size the card is drawing at, in its own pixels, written by the view as it lays out.</summary>
    private double _cardWidth;
    private double _cardHeight;

    private bool _hasPointer;
    private double _pointerLeft;
    private double _pointerTop;

    /// <summary>
    /// Whether this stream is carrying a pointer position at all.
    /// False for every cursor mode but the one that sends it,
    /// false for a format whose bitstream carries none,
    /// and false while the pointer is off the publisher's captured screen.
    /// A pointer that has left is not at its last position.
    /// </summary>
    public bool HasPointer { get => _hasPointer; private set => Set(ref _hasPointer, value); }

    /// <summary>Where the marker's tip sits over the card, in the card's own pixels.</summary>
    public double PointerLeft { get => _pointerLeft; private set => Set(ref _pointerLeft, value); }

    public double PointerTop { get => _pointerTop; private set => Set(ref _pointerTop, value); }

    /// <summary>
    /// Size this card is drawing at, written by the view as it arranges.
    /// The one fact the view knows and the view model cannot read, hence the one input the view writes.
    /// </summary>
    public void SetPictureSize(double width, double height)
    {
        _cardWidth = width;
        _cardHeight = height;
        Place();
    }

    /// <summary>
    /// Takes one position, or none.
    /// Its own entry point rather than part of <see cref="Apply"/>: positions arrive as fast as frames do,
    /// and a render pass each would re-read the whole tile to move one marker.
    /// </summary>
    public void Point(PointerPosition? at)
    {
        _at = at;
        Place();
    }

    /// <summary>
    /// Puts the marker where the position says, on the picture rather than on the card.
    ///
    /// The host letterboxes the picture to the stream's shape, so the card is the picture only where the two
    /// share an aspect, and a fraction of the card would drift into the bars everywhere else.
    /// The picture is the largest rectangle of the stream's shape that fits, centred, which is what
    /// a letterboxing host draws and what a host that needs no bars draws too.
    ///
    /// Nothing is drawn until a frame has stated a size: a shape guessed here would place the marker against
    /// a picture nobody has seen.
    /// </summary>
    private void Place()
    {
        var at = _at;
        if (at is null || !at.Visible || _cardWidth <= 0 || _cardHeight <= 0
            || _report.Width <= 0 || _report.Height <= 0)
        {
            HasPointer = false;
            return;
        }

        var aspect = (double)_report.Width / _report.Height;
        var width = System.Math.Min(_cardWidth, _cardHeight * aspect);
        var height = width / aspect;

        // No offset onto the shape: the marker's tip is its origin, and the position is where the tip goes
        // (Design/Pointer.axaml).
        HasPointer = true;
        PointerLeft = ((_cardWidth - width) / 2) + (at.X * width);
        PointerTop = ((_cardHeight - height) / 2) + (at.Y * height);
    }

    /// <summary>
    /// Follows this stream's pointer for as long as the frame subscription runs.
    ///
    /// Relay tiles alone.
    /// The publish's own preview draws a position read off this machine's capture, which the insights screen
    /// subscribes to once for the whole screen, and a monitor preview is a picture nothing published.
    /// </summary>
    private void FollowPointer(CancellationToken cancellation)
    {
        if (!_source.IsRelay)
        {
            return;
        }
        _ = PointerAsync(cancellation);
    }

    /// <summary>
    /// Reconnect wait after the pointer stream drops, matching the session's own.
    /// A picture goes on being drawn while this reconnects, the position being no part of it.
    /// </summary>
    private static readonly TimeSpan PointerReconnectDelay = TimeSpan.FromSeconds(1);

    private async Task PointerAsync(CancellationToken cancellation)
    {
        var stream = new StreamRef { StreamName = Name, Transport = Transport };

        while (!cancellation.IsCancellationRequested)
        {
            try
            {
                await foreach (var at in _backend.SubscribePointerAsync(stream, cancellation)
                                   .ConfigureAwait(false))
                {
                    _dispatch(() => Point(at));
                }
            }
            catch (OperationCanceledException)
            {
                return;
            }
            catch (BackendUnavailableException)
            {
                // The tile already says why it is dark, so this loop says nothing.
            }

            // A marker frozen where a dropped stream left it is the one reading that is certainly wrong.
            _dispatch(() => Point(null));

            try
            {
                await Task.Delay(PointerReconnectDelay, cancellation).ConfigureAwait(false);
            }
            catch (OperationCanceledException)
            {
                return;
            }
        }
    }
}
