using Avalonia;
using Avalonia.Controls;
using Avalonia.Platform;
using Avalonia.Rendering.Composition;
using Avalonia.Threading;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Viewer.Tile.Model;

namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// One decoded stream, drawn from the GPU memory it was decoded into.
///
/// <b>Nothing here reads a pixel.</b> The backend decodes into a shared texture, names it on
/// the frame channel, and this imports that name into the compositor's own device and draws
/// it. No frame crosses a message, no frame enters system memory, and no frame is copied by
/// this process at all (<c>docs/viewer-architecture.md</c>, "The frame channel").
///
/// <b>It is not a <see cref="NativeControlHost"/>, and that is the design decision this
/// control exists to carry.</b> Handing GStreamer a window handle is the easy path and the
/// wrong one: a native child window draws above every Avalonia control, so a figure or a
/// menu over the video would disappear behind it. A composition surface is a visual among
/// visuals, so what is drawn over a tile stays over it (<c>avalonia/README.md</c>).
///
/// <b>The loan is what makes it correct.</b> Each frame arrives as a slot the backend has
/// lent, and the slot goes back only after the compositor has taken it - which is what
/// <see cref="CompositionDrawingSurface.UpdateWithKeyedMutexAsync"/> waits for. A tile that
/// is slow therefore costs frames the backend drops, and never a half-written picture and
/// never a stalled pipeline.
/// </summary>
public sealed class StreamTile : Control
{
    /// <summary>
    /// Where the frames come from and where this tile reports what it drew. A tile with no
    /// source draws nothing and says nothing, which is the state of one whose row has not
    /// been asked for yet.
    /// </summary>
    public static readonly StyledProperty<IFrameSource?> SourceProperty =
        AvaloniaProperty.Register<StreamTile, IFrameSource?>(nameof(Source));

    /// <summary>
    /// The compositor objects this tile draws through. They are made once per attach and
    /// dropped on detach, because a composition visual belongs to the compositor of the tree
    /// it is in and a tile can be moved between trees.
    /// </summary>
    private CompositionDrawingSurface? _surface;

    /// <summary>
    /// The visual the surface is drawn by. It is kept because the size lives here rather than
    /// on the surface: a drawing surface holds pixels and a visual holds where they go.
    /// </summary>
    private CompositionSurfaceVisual? _visual;

    /// <summary>Whether this tile is in a visual tree, so a source swap knows whether to restart.</summary>
    private bool _attached;

    /// <summary>
    /// The imported slots of the current pool, by slot index. Each is imported once and drawn
    /// from many times, which is the whole point of the pool: a per-frame import would be a
    /// per-frame trip through the graphics driver.
    /// </summary>
    private readonly List<ICompositionImportedGpuImage> _slots = [];

    /// <summary>The pool the imports belong to, so a frame of an older one is not drawn from a newer slot.</summary>
    private ulong _generation;
    private uint _acquireKey;
    private uint _releaseKey;

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
    /// three texture allocations and a renegotiation of the branch. A grid that rearranges - a
    /// window dragged, a tile focused, a stream joining - moves every tile's exact size; rounded
    /// onto a ladder, most of those moves ask for the size that was already in force and cost
    /// nothing at all.
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
    /// Sizes the drawn visual and tells the backend how many pixels this tile needs.
    ///
    /// The size sent is in device pixels rather than in layout units, because it is a count of
    /// pixels the scaler fixates against and a 200-unit tile on a 200% display needs 400 of them.
    /// It is rounded up onto the ladder and sent once the size has settled, and only when it
    /// differs from the one in force: all three exist because writing the pipeline's filter
    /// renegotiates the branch and re-announces the pool behind it.
    ///
    /// The visual itself is sized immediately. That is a local draw and costs nothing, so a tile
    /// follows the layout exactly while what it asks the backend for lags behind it.
    /// </summary>
    private void Fit(Size size)
    {
        if (_visual is null)
        {
            return;
        }

        _visual.Size = new Avalonia.Vector(size.Width, size.Height);

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
    /// Opens the compositor objects and starts the subscription.
    ///
    /// Everything after the first line is asynchronous and is allowed to be: the tile is a
    /// control that draws nothing until frames arrive, so a subscription that takes a moment
    /// to open is a tile that is briefly empty rather than a window that waits.
    /// </summary>
    private void Start()
    {
        var source = Source;
        if (source is null || _cancel is not null)
        {
            return;
        }

        var self = ElementComposition.GetElementVisual(this);
        if (self is null)
        {
            return;
        }

        var compositor = self.Compositor;
        _surface = compositor.CreateDrawingSurface();

        _visual = compositor.CreateSurfaceVisual();
        _visual.Surface = _surface;
        _visual.Size = new Avalonia.Vector(Bounds.Width, Bounds.Height);
        ElementComposition.SetElementChildVisual(this, _visual);

        _cancel = new CancellationTokenSource();
        _ = RunAsync(compositor, source, _cancel.Token);
    }

    /// <summary>
    /// Ends the subscription and drops everything it imported.
    ///
    /// The order matters and is the one place this control has to be careful: the imports are
    /// disposed before the channel closes, because the backend frees the pool as the call ends
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

        ElementComposition.SetElementChildVisual(this, null);
        _surface = null;
        _visual = null;
        _asked = default;
        _generation = 0;
        // Back to what the field was constructed with rather than to the struct's default:
        // a tile is stopped and started again whenever its row is replaced, and a default
        // here would put the null notice back that TileReport.Nothing exists to keep out.
        _reported = TileReport.Nothing;
    }

    /// <summary>
    /// The subscription, from opening it to the stream ending.
    ///
    /// It runs on the UI loop by construction - it is started from there and every await
    /// returns to it - which is what makes the imports and the reports safe to make from
    /// inside it without marshalling each one.
    /// </summary>
    private async Task RunAsync(Compositor compositor, IFrameSource source, CancellationToken cancellation)
    {
        try
        {
            var interop = await compositor.TryGetCompositionGpuInterop().ConfigureAwait(true);
            if (interop is null)
            {
                Report(_reported with { Notice = "This window's renderer cannot import a shared texture." });
                return;
            }

            await using var channel = await source.OpenAsync(cancellation).ConfigureAwait(true);
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
                        await ImportAsync(interop, message.Pool).ConfigureAwait(true);
                        break;
                    case FrameEvent.EventOneofCase.Ready:
                        await DrawAsync(channel, message.Ready).ConfigureAwait(true);
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
            // A failure to import is the one case worth showing as it stands: it names a
            // driver or a handle type, and it is the difference between a tile that is empty
            // because nothing is publishing and one that is empty because this machine cannot
            // open what the backend lent it.
            Report(_reported with { Notice = e.Message });
        }
        finally
        {
            _channel = null;
            await ReleaseAsync().ConfigureAwait(true);
        }
    }

    /// <summary>
    /// Imports one pool's slots, replacing whatever the previous one left.
    ///
    /// A pool is announced once per negotiation and its slots are imported once each. What is
    /// checked here is that this machine can open the handle type at all: a compositor that
    /// does not list it would fail per frame with the same reason, and saying it once beside
    /// the tile is the honest version of that.
    /// </summary>
    private async Task ImportAsync(ICompositionGpuInterop interop, FramePool pool)
    {
        await ReleaseAsync().ConfigureAwait(true);

        var handleType = HandleTypeOf(pool.HandleType);
        if (handleType is null || !interop.SupportedImageHandleTypes.Contains(handleType))
        {
            Report(_reported with
            {
                Notice = "This window's renderer cannot open the kind of shared frame this machine decodes into.",
            });
            return;
        }

        var properties = new PlatformGraphicsExternalImageProperties
        {
            Width = (int)pool.Width,
            Height = (int)pool.Height,
            Format = FormatOf(pool.Format),
            MemorySize = pool.MemorySize,
            TopLeftOrigin = pool.TopLeftOrigin,
        };

        foreach (var slot in pool.Slots)
        {
            _slots.Add(interop.ImportImage(
                new PlatformHandle(new IntPtr((long)slot.Handle), handleType),
                properties));
        }

        _generation = pool.Generation;
        _acquireKey = pool.ConsumerKey;
        _releaseKey = pool.ProducerKey;

        Report(_reported with
        {
            Width = (int)pool.Width,
            Height = (int)pool.Height,
            Notice = "",
        });
    }

    /// <summary>
    /// Draws one lent slot and hands it back.
    ///
    /// The await is the flow control. It completes when the compositor has taken the texture,
    /// which is when the slot is genuinely free, so the release that follows is a statement
    /// rather than a guess - and a tile the compositor is slow to serve stops asking for
    /// frames instead of overwriting one it is still drawing.
    ///
    /// A frame naming a pool that has been replaced is dropped. Its slot is released all the
    /// same, because the backend recognises the stale generation and discards it, and not
    /// releasing would be this side deciding to keep something it cannot draw.
    /// </summary>
    private async Task DrawAsync(FrameChannel channel, FrameReady ready)
    {
        if (ready.Generation == _generation && ready.Slot < _slots.Count)
        {
            await _surface!.UpdateWithKeyedMutexAsync(_slots[(int)ready.Slot], _acquireKey, _releaseKey)
                .ConfigureAwait(true);

            Report(_reported with { Frames = ready.Serial, Dropped = ready.Dropped, Notice = "" });
        }

        await channel.ReleaseAsync(ready.Generation, ready.Slot, ready.Serial).ConfigureAwait(true);
    }

    /// <summary>
    /// Drops the imports of the pool that is no longer current.
    ///
    /// The dispose is awaited rather than fired off, because an import is released on the
    /// render thread and the backend frees the texture behind it as soon as the call ends: a
    /// release still in flight when that happens is a release against memory that is gone.
    /// </summary>
    private async Task ReleaseAsync()
    {
        foreach (var slot in _slots)
        {
            await slot.DisposeAsync().ConfigureAwait(true);
        }

        _slots.Clear();
        _generation = 0;
    }

    /// <summary>
    /// Tells the source what this tile is drawing, and only when it moved.
    ///
    /// Idempotent by construction: the report is a record, so two passes over one state
    /// compare equal and the second raises nothing (<c>docs/development-principles.md</c>).
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

    /// <summary>
    /// The compositor's name for a handle type the backend can lend, and null for one no
    /// import here knows about.
    ///
    /// This is the one place the contract's identifiers meet the toolkit's, and it is a map
    /// rather than an assumption: which handle types a backend imports differs between two
    /// graphics backends on one operating system, which is why the supported list is asked
    /// for rather than derived from the platform.
    /// </summary>
    private static string? HandleTypeOf(FrameHandleType type) => type switch
    {
        FrameHandleType.D3D11GlobalShared => KnownPlatformGraphicsExternalImageHandleTypes.D3D11TextureGlobalSharedHandle,
        _ => null,
    };

    private static PlatformGraphicsExternalImageFormat FormatOf(FrameFormat format) => format switch
    {
        FrameFormat.B8G8R8A8Unorm => PlatformGraphicsExternalImageFormat.B8G8R8A8UNorm,
        _ => PlatformGraphicsExternalImageFormat.R8G8B8A8UNorm,
    };
}
