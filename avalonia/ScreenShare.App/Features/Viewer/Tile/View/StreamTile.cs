using Avalonia;
using Avalonia.Controls;
using Avalonia.Threading;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Viewer.Tile.Model;

namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// One decoded stream, drawn from the GPU memory it was decoded into.
///
/// <b>Nothing here reads a pixel.</b> The backend decodes into shared GPU memory, names it on
/// the frame channel, and a surface imports that name and draws it. No frame crosses a
/// message, no frame enters system memory, and no frame is copied by this process at all
/// (<c>docs/viewer-architecture.md</c>, "The frame channel").
///
/// <b>What is here is the subscription and the loan.</b> Opening the channel, asking for a
/// render size, deciding which slot is drawn next, handing slots back and saying what this
/// tile is doing are the same on every machine. How a slot becomes a picture is not, and it is
/// the surface's whole job (<see cref="ITileSurface"/>): the pool says what its handles are and
/// the tile puts the matching surface in front of itself.
///
/// <b>Each frame is a loan.</b> A slot is the consumer's until it is handed back, and it is
/// handed back only once the surface says the draw has finished. A tile that is slow therefore
/// costs frames the backend drops, and never a half-written picture and never a stalled
/// pipeline.
/// </summary>
public sealed class StreamTile : Decorator
{
    /// <summary>
    /// Where the frames come from and where this tile reports what it drew. A tile with no
    /// source draws nothing and says nothing, which is the state of one whose row has not been
    /// asked for yet.
    /// </summary>
    public static readonly StyledProperty<IFrameSource?> SourceProperty =
        AvaloniaProperty.Register<StreamTile, IFrameSource?>(nameof(Source));

    /// <summary>Whether this tile is in a visual tree, so a source swap knows whether to restart.</summary>
    private bool _attached;

    /// <summary>The pool the surface imported, so a frame of an older one is not drawn from a newer slot.</summary>
    private ulong _generation;

    /// <summary>The running subscription, cancelled on detach.</summary>
    private CancellationTokenSource? _cancel;

    /// <summary>What has been reported, so a report that changed nothing raises no pass.</summary>
    private TileReport _reported = TileReport.Nothing;

    /// <summary>The size last sent to the backend, in device pixels, so an unchanged size renegotiates nothing.</summary>
    private PixelSize _asked;

    public IFrameSource? Source
    {
        get => GetValue(SourceProperty);
        set => SetValue(SourceProperty, value);
    }

    protected override void OnAttachedToVisualTree(VisualTreeAttachmentEventArgs e)
    {
        base.OnAttachedToVisualTree(e);
        _attached = true;
        Start();
    }

    protected override void OnDetachedFromVisualTree(VisualTreeAttachmentEventArgs e)
    {
        base.OnDetachedFromVisualTree(e);
        _attached = false;
        Stop();
    }

    protected override void OnPropertyChanged(AvaloniaPropertyChangedEventArgs change)
    {
        base.OnPropertyChanged(change);

        if (change.Property == SourceProperty && _attached)
        {
            // A tile whose row was replaced is a tile drawing a different stream, so the
            // subscription is not reused: the pool belongs to the decode it was lent by.
            Stop();
            Start();
        }
    }

    protected override Size ArrangeOverride(Size size)
    {
        var arranged = base.ArrangeOverride(size);
        Fit(arranged);
        return arranged;
    }

    /// <summary>
    /// The heights a tile ever asks for.
    ///
    /// <b>A ladder rather than the exact size, because a size that moved re-announces a pool.</b>
    /// The backend allocates its slots at the size it was asked for, so every distinct ask costs
    /// three allocations and a renegotiation of the branch. A grid that rearranges - a window
    /// dragged, a tile focused, a stream joining - moves every tile's exact size; rounded onto a
    /// ladder, most of those moves ask for the size that was already in force and cost nothing
    /// at all.
    ///
    /// What is lost is a scaler fixating exactly on the tile: a tile between two rungs is handed
    /// frames a little larger than it draws and scales them down at draw time, which is a
    /// resample the GPU was doing anyway.
    /// </summary>
    private static readonly int[] Ladder = [360, 540, 720, 900, 1080, 1440, 2160];

    /// <summary>
    /// How long the size has to hold still before it is sent.
    ///
    /// A window being dragged produces an arrange pass per frame, and each one that crossed a
    /// rung would re-announce a pool. Waiting for the drag to settle turns a resize into one
    /// renegotiation rather than one per rung crossed on the way.
    /// </summary>
    private static readonly TimeSpan SettleDelay = TimeSpan.FromMilliseconds(250);

    /// <summary>The timer that sends the size once it has stopped moving.</summary>
    private DispatcherTimer? _settle;

    /// <summary>The size the settle timer will send, in device pixels.</summary>
    private PixelSize _pending;

    /// <summary>
    /// Tells the backend how many pixels this tile needs.
    ///
    /// The size sent is in device pixels rather than in layout units, because it is a count of
    /// pixels the scaler fixates against and a 200-unit tile on a 200% display needs 400 of
    /// them. It is rounded up onto the ladder and sent once the size has settled, and only when
    /// it differs from the one in force: all three exist because writing the pipeline's filter
    /// renegotiates the branch and re-announces the pool behind it.
    ///
    /// Nothing here sizes what is drawn. The surface is this control's child and the layout
    /// gives it the whole tile, so the picture follows the arrangement exactly while what the
    /// backend is asked for lags behind it.
    /// </summary>
    private void Fit(Size size)
    {
        var scaling = TopLevel.GetTopLevel(this)?.RenderScaling ?? 1;
        var wanted = new PixelSize(
            Math.Max(0, (int)Math.Ceiling(size.Width * scaling)),
            Math.Max(0, (int)Math.Ceiling(size.Height * scaling)));
        if (wanted.Width == 0 || wanted.Height == 0)
        {
            return;
        }

        // The rung is chosen on the height and the width follows the tile's own shape, so two
        // tiles of one height and different aspect ratios ask for the sizes they actually draw
        // rather than both being padded to a square.
        var height = Rung(wanted.Height);
        var pixels = new PixelSize(
            Math.Max(1, (int)Math.Round(height * ((double)wanted.Width / wanted.Height))),
            height);

        if (pixels == _asked || pixels == _pending)
        {
            return;
        }

        _pending = pixels;
        _settle ??= new DispatcherTimer { Interval = SettleDelay };
        _settle.Tick -= OnSettled;
        _settle.Tick += OnSettled;
        _settle.Stop();
        _settle.Start();
    }

    /// <summary>Sends the size the tile settled at, once it has stopped moving.</summary>
    private void OnSettled(object? sender, EventArgs e)
    {
        _settle?.Stop();

        if (_pending == _asked || _pending.Width == 0)
        {
            return;
        }

        _asked = _pending;
        _ = _channel?.RenderSizeAsync(_asked.Width, _asked.Height);
    }

    /// <summary>
    /// The rung a height is asked for at: the first one that covers it, or the top rung for a
    /// tile larger than the ladder goes.
    /// </summary>
    private static int Rung(int height)
    {
        foreach (var rung in Ladder)
        {
            if (height <= rung)
            {
                return rung;
            }
        }

        return Ladder[^1];
    }

    /// <summary>The open subscription, held so the arrange pass can report a size on it.</summary>
    private FrameChannel? _channel;

    /// <summary>
    /// Starts the subscription.
    ///
    /// Everything after the first line is asynchronous and is allowed to be: the tile is a
    /// control that draws nothing until frames arrive, so a subscription that takes a moment to
    /// open is a tile that is briefly empty rather than a window that waits.
    /// </summary>
    private void Start()
    {
        var source = Source;
        if (source is null || _cancel is not null)
        {
            return;
        }

        _cancel = new CancellationTokenSource();
        _ = RunAsync(source, _cancel.Token);
    }

    /// <summary>
    /// Ends the subscription and drops everything it imported.
    ///
    /// The order matters and is the one place this control has to be careful: the surface is
    /// dropped before the channel closes, because the backend frees the pool as the call ends
    /// and an import outliving it would name memory that is gone.
    /// </summary>
    private void Stop()
    {
        // The settle timer goes first. It would otherwise fire against a channel that is being
        // closed, and a size sent onto a subscription that has ended is a call into a pool the
        // backend has already freed.
        _settle?.Stop();
        _asked = default;
        _pending = default;

        _cancel?.Cancel();
        _cancel?.Dispose();
        _cancel = null;

        _generation = 0;
        // Back to what the field was constructed with rather than to the struct's default: a
        // tile is stopped and started again whenever its row is replaced, and a default here
        // would put the null notice back that TileReport.Nothing exists to keep out.
        _reported = TileReport.Nothing;
    }

    /// <summary>
    /// The subscription, from opening it to the stream ending.
    ///
    /// It runs on the UI loop by construction - it is started from there and every await
    /// returns to it - which is what makes the imports and the reports safe to make from inside
    /// it without marshalling each one.
    /// </summary>
    private async Task RunAsync(IFrameSource source, CancellationToken cancellation)
    {
        // Both belong to this run rather than to the control, and that is what makes a tile
        // whose row was replaced safe: the subscription that is ending tears down what it
        // opened, while the one that started in its place holds its own.
        FrameChannel? channel = null;
        ITileSurface? surface = null;

        try
        {
            channel = await source.OpenAsync(cancellation).ConfigureAwait(true);
            _channel = channel;

            // The size is sent before the first frame, so the pipeline scales to this tile
            // rather than converting a whole desktop and having it thrown away at draw time.
            _asked = default;
            Fit(Bounds.Size);

            await foreach (var message in channel.ReadAsync(cancellation).ConfigureAwait(true))
            {
                switch (message.EventCase)
                {
                    case FrameEvent.EventOneofCase.Pool:
                        surface = await ImportAsync(surface, message.Pool, cancellation).ConfigureAwait(true);
                        break;
                    case FrameEvent.EventOneofCase.Ready:
                        await DrawAsync(surface, channel, message.Ready, cancellation).ConfigureAwait(true);
                        break;
                    case FrameEvent.EventOneofCase.End:
                        Report(_reported with { Notice = message.End.Message });
                        return;
                }
            }
        }
        catch (OperationCanceledException)
        {
            // The tile was detached. Nothing to report: the control is on its way off screen.
        }
        catch (BackendUnavailableException e)
        {
            Report(_reported with { Notice = e.Message });
        }
        catch (Exception e)
        {
            // A failure to import is the one case worth showing as it stands: it names a driver
            // or a handle type, and it is the difference between a tile that is empty because
            // nothing is publishing and one that is empty because this machine cannot open what
            // the backend lent it.
            Report(_reported with { Notice = e.Message });
        }
        finally
        {
            _channel = null;
            // The surface goes before the channel. The backend frees the pool as the call ends,
            // and an import outliving it would name memory that is gone.
            await ReleaseAsync(surface).ConfigureAwait(true);
            if (channel is not null)
            {
                await channel.DisposeAsync().ConfigureAwait(true);
            }
        }
    }

    /// <summary>
    /// Imports one pool, on a surface that can open the kind of handle it lends.
    ///
    /// The surface is made from the pool rather than from the platform, and it is remade when a
    /// pool arrives that needs another kind: which handle types a machine opens is a property of
    /// its renderer, and the pool is where the backend says what it made.
    /// </summary>
    private async Task<ITileSurface?> ImportAsync(
        ITileSurface? surface,
        FramePool pool,
        CancellationToken cancellation)
    {
        _generation = 0;

        surface = await SurfaceFor(surface, pool.HandleType).ConfigureAwait(true);
        if (surface is null)
        {
            Report(_reported with
            {
                Notice = "This window's renderer cannot open the kind of shared frame this machine decodes into.",
            });
            return null;
        }

        var notice = await surface.ImportAsync(pool, cancellation).ConfigureAwait(true);
        if (notice is not null)
        {
            Report(_reported with { Notice = notice });
            return surface;
        }

        _generation = pool.Generation;
        Report(_reported with
        {
            Width = (int)pool.Width,
            Height = (int)pool.Height,
            Notice = "",
        });
        return surface;
    }

    /// <summary>
    /// The surface for this kind of handle: the one already in front of the tile where it draws
    /// this kind, and a new one where it does not. Null is a handle type nothing here opens.
    ///
    /// A pool is re-announced on every renegotiation, so the reuse is what keeps a resize from
    /// building a renderer's context and shaders again for the same kind of frame.
    /// </summary>
    private async Task<ITileSurface?> SurfaceFor(ITileSurface? surface, FrameHandleType type)
    {
        if (surface is not null && surface.Handle == type)
        {
            return surface;
        }

        await ReleaseAsync(surface).ConfigureAwait(true);

        var made = TileSurfaces.For(type);
        Assert.That(made is null || made.Handle == type,
            "a surface draws the handle type it was made for", type.ToString());

        Child = made?.View;
        return made;
    }

    /// <summary>
    /// Draws one lent slot and hands it back.
    ///
    /// The await is the flow control. It completes when the surface has finished with the slot,
    /// which is when the slot is genuinely free, so the release that follows is a statement
    /// rather than a guess - and a tile whose renderer is slow stops asking for frames instead
    /// of drawing one it is still reading.
    ///
    /// A frame naming a pool that has been replaced is dropped. Its slot is released all the
    /// same, because the backend recognises the stale generation and discards it, and not
    /// releasing would be this side deciding to keep something it cannot draw.
    /// </summary>
    private async Task DrawAsync(
        ITileSurface? surface,
        FrameChannel channel,
        FrameReady ready,
        CancellationToken cancellation)
    {
        if (ready.Generation == _generation && surface is not null)
        {
            await surface.DrawAsync(ready.Slot, cancellation).ConfigureAwait(true);
            Report(_reported with { Frames = ready.Serial, Dropped = ready.Dropped, Notice = "" });
        }

        await channel.ReleaseAsync(ready.Generation, ready.Slot, ready.Serial).ConfigureAwait(true);
    }

    /// <summary>
    /// Drops the surface and everything it imported.
    ///
    /// The dispose is awaited rather than fired off, because an import is released on the render
    /// thread and the backend frees the memory behind it as soon as the call ends: a release
    /// still in flight when that happens is a release against memory that is gone.
    /// </summary>
    private async Task ReleaseAsync(ITileSurface? surface)
    {
        _generation = 0;
        if (surface is null)
        {
            return;
        }

        // Only where this is still the surface on screen. A run that is ending after its
        // replacement started would otherwise take the new one's picture with it.
        if (ReferenceEquals(Child, surface.View))
        {
            Child = null;
        }
        await surface.DisposeAsync().ConfigureAwait(true);
    }

    /// <summary>
    /// Tells the source what this tile is drawing, and only when it moved.
    ///
    /// Idempotent by construction: the report is a record, so two passes over one state compare
    /// equal and the second raises nothing (<c>docs/development-principles.md</c>).
    /// </summary>
    private void Report(TileReport report)
    {
        if (report == _reported)
        {
            return;
        }

        _reported = report;
        Dispatcher.UIThread.VerifyAccess();
        Source?.Report(report);
    }
}
