using Avalonia;
using Avalonia.Controls;
using Avalonia.Threading;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Viewer.Tile.Model;

namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// One decoded stream, drawn out of the GPU memory it was decoded into.
///
/// No pixel is read.
/// The decoder writes shared GPU memory, the frame channel carries its name, a surface imports that name:
/// nothing in a message, nothing in system memory, nothing copied by this process
/// (<c>docs/viewer-architecture.md</c>, "The frame channel").
///
/// What lives here is the subscription and the loan: opening the channel, asking for a render size, picking
/// the slot to draw, giving slots back, and reporting.
/// Turning a slot into a picture is <see cref="ITileSurface"/>'s job, and the pool names which one does it.
///
/// Every frame is a loan.
/// A slot belongs to the consumer until it goes back, and it goes back only once the surface reports the draw
/// complete, so a slow tile pays in frames the backend drops rather than in a torn picture or a stalled
/// pipeline.
/// </summary>
public sealed class StreamTile : Decorator
{
    /// <summary>
    /// Origin of the frames, and sink for what this tile reports.
    /// null neither draws nor reports: the grid slot of a stream popped out into its own window, and a row
    /// nothing has asked for.
    /// </summary>
    public static readonly StyledProperty<IFrameSource?> SourceProperty =
        AvaloniaProperty.Register<StreamTile, IFrameSource?>(nameof(Source));

    /// <summary>Presence in a visual tree. A source swap restarts only while it holds.</summary>
    private bool _attached;

    /// <summary>
    /// Generation of the imported pool, zero while none is.
    /// A frame from any other generation is not drawn.
    /// </summary>
    private ulong _generation;

    /// <summary>Cancellation of the live subscription. Non-null exactly while one runs.</summary>
    private CancellationTokenSource? _cancel;

    /// <summary>Report last handed to the source, so an identical one raises no pass.</summary>
    private TileReport _reported = TileReport.Nothing;

    /// <summary>Size the backend already has, in device pixels. Repeating it renegotiates nothing.</summary>
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
            // A new row is a different stream, and a pool belongs to the decode that lent it, so the
            // subscription is torn down instead of reused.
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
    /// Heights this control will ever ask for.
    ///
    /// Rungs instead of exact sizes, because a moved size re-announces a pool: the backend allocates its
    /// slots at the size it was given, so each distinct ask costs an allocation and a renegotiated branch.
    /// Rearrangement moves every tile's exact size, whether a window was dragged, a tile focused or a stream
    /// joined, and rounding up lands most of those moves on the size already in force.
    ///
    /// The price is a scaler that stops short of the tile's exact size: between two rungs the frames arrive
    /// slightly large and are scaled at draw time, a resample the GPU performs regardless.
    /// </summary>
    private static readonly int[] Ladder = [360, 540, 720, 900, 1080, 1440, 2160];

    /// <summary>
    /// Quiet period a size waits out before it is sent.
    /// A dragged window arranges once per frame, and each rung crossed on the way would re-announce a pool.
    /// </summary>
    private static readonly TimeSpan SettleDelay = TimeSpan.FromMilliseconds(250);

    private DispatcherTimer? _settle;

    /// <summary>Size awaiting the settle timer, in device pixels.</summary>
    private PixelSize _pending;

    /// <summary>
    /// States how many pixels this tile needs.
    ///
    /// Device pixels rather than layout units: the scaler fixates against a pixel count, and a 200-unit tile
    /// on a 200% display wants 400 of them.
    /// Rounded onto a rung, held until the size settles, and skipped where it matches the size in force,
    /// because writing the pipeline's filter renegotiates the branch and re-announces its pool.
    ///
    /// Nothing here sizes the picture.
    /// The surface is a child and layout hands it the whole tile, so the drawing tracks the arrangement while
    /// the ask trails it.
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

        // Height picks the rung and width follows the tile's own shape, so two tiles of equal height and
        // different ratios ask for what each draws instead of both being padded square.
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

    /// <summary>Lowest rung covering the height, and the topmost rung for anything above the ladder.</summary>
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

    /// <summary>Live subscription, kept so an arrange pass has somewhere to send a size.</summary>
    private FrameChannel? _channel;

    /// <summary>
    /// Opens the subscription.
    /// Asynchronous after the first line, which costs nothing: a tile paints nothing until frames land, so a
    /// slow open is a briefly blank tile rather than a stalled window.
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
    /// Ends the subscription.
    /// The run unwinds its own channel and surface, leaving this to the state the control keeps alongside
    /// them.
    /// </summary>
    private void Stop()
    {
        // Timer first.
        // Left running it fires at a closing channel, and a size on a finished subscription reaches a pool
        // the backend has freed.
        _settle?.Stop();
        _asked = default;
        _pending = default;

        _cancel?.Cancel();
        _cancel?.Dispose();
        _cancel = null;

        _generation = 0;
        // TileReport.Nothing, not the struct's default: a replaced row stops and starts this control, and
        // that default carries the null notice the constant exists to keep out.
        _reported = TileReport.Nothing;
    }

    /// <summary>
    /// The subscription's whole life, from the open to the end of the stream.
    ///
    /// It sits on the UI loop by construction, launched from there with every await resuming there, which is
    /// what lets it import and report inline instead of marshalling each one.
    /// </summary>
    private async Task RunAsync(IFrameSource source, CancellationToken cancellation)
    {
        // Owned by the run rather than by the control, which is what makes a replaced row safe: the ending
        // subscription tears down what it opened while its successor holds its own.
        FrameChannel? channel = null;
        ITileSurface? surface = null;

        try
        {
            channel = await source.OpenAsync(cancellation).ConfigureAwait(true);
            _channel = channel;

            // Ahead of the first frame, so the pipeline converts at this tile's size instead of producing a
            // whole desktop to be thrown away at draw time.
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
            // Detached.
            // Nothing to say: the control is leaving the screen.
        }
        catch (BackendUnavailableException e)
        {
            Report(_reported with { Notice = e.Message });
        }
        catch (Exception e)
        {
            // A failed import is shown as it stands: it names a driver or a handle type, which separates a
            // tile blank because nobody is publishing from one blank because this machine cannot open what
            // was lent to it.
            Report(_reported with { Notice = e.Message });
        }
        finally
        {
            _channel = null;
            // Surface ahead of channel.
            // Closing the call frees the pool, so an import outliving it would name memory that is gone.
            await ReleaseAsync(surface).ConfigureAwait(true);
            if (channel is not null)
            {
                await channel.DisposeAsync().ConfigureAwait(true);
            }
        }
    }

    /// <summary>
    /// Takes on one pool, through a surface that opens its kind of handle.
    ///
    /// The surface follows the pool rather than the platform and is rebuilt when a pool wants another kind:
    /// what a machine can open is its renderer's property, and the pool is where the backend states what it
    /// allocated.
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
    /// Surface for a handle type: the one on screen where it already draws that kind, a fresh one otherwise,
    /// null where nothing imports it.
    ///
    /// Every renegotiation re-announces the pool, so the reuse is what stops a resize rebuilding a renderer's
    /// context and shaders for the same kind of frame.
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
    /// Draws one lent slot and gives it back.
    ///
    /// Flow control lives in that await.
    /// It resolves when the surface has finished with the slot, so the release behind it reports rather than
    /// assumes, and a tile with a slow renderer stops asking for frames instead of drawing over one it is
    /// reading.
    ///
    /// A frame from a replaced pool is dropped, its slot released regardless: the backend spots the stale
    /// generation and discards the release, and withholding it would mean keeping something undrawable.
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
    /// Discards the surface and its imports.
    ///
    /// Awaited, not fired off: the release runs on the render thread while the backend frees the memory as
    /// the call ends, and one still in flight would land on memory that is gone.
    /// </summary>
    private async Task ReleaseAsync(ITileSurface? surface)
    {
        _generation = 0;
        if (surface is null)
        {
            return;
        }

        // Only while this is still the surface on screen.
        // A run ending after its successor started would otherwise take the new picture away.
        if (ReferenceEquals(Child, surface.View))
        {
            Child = null;
        }
        await surface.DisposeAsync().ConfigureAwait(true);
    }

    /// <summary>
    /// Reports what this tile is drawing, and only on a change.
    ///
    /// Idempotent by construction: a report is a record, so a second pass over one state compares equal and
    /// raises nothing (<c>docs/development-principles.md</c>).
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
