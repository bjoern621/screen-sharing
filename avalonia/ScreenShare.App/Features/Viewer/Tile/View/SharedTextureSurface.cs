using Avalonia;
using Avalonia.Controls;
using Avalonia.Platform;
using Avalonia.Rendering.Composition;
using ScreenShare.Api.V1;

namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// The surface for a handle the compositor imports itself: a shared texture, named to
/// Avalonia's own device and drawn as a composition visual.
///
/// <b>Nothing here reads a pixel.</b> The backend decodes into a shared texture, names it on
/// the frame channel, and this hands that name to the compositor. No frame crosses a message,
/// no frame enters system memory, and no frame is copied by this process
/// (<c>docs/viewer-architecture.md</c>, "The frame channel").
///
/// <b>It is not a <see cref="NativeControlHost"/>, and that is the design decision this
/// control exists to carry.</b> Handing GStreamer a window handle is the easy path and the
/// wrong one: a native child window draws above every Avalonia control, so a figure or a menu
/// over the video would disappear behind it. A composition surface is a visual among visuals,
/// so what is drawn over a tile stays over it (<c>avalonia/README.md</c>).
///
/// <b>The loan is what makes it correct.</b> Each frame arrives as a slot the backend has
/// lent, and the slot goes back only after the compositor has taken it - which is what
/// <see cref="CompositionDrawingSurface.UpdateWithKeyedMutexAsync"/> waits for. A tile that is
/// slow therefore costs frames the backend drops, and never a half-written picture and never a
/// stalled pipeline.
/// </summary>
internal sealed class SharedTextureSurface : Control, ITileSurface
{
    /// <summary>
    /// The compositor objects this surface draws through. They are made once per attach and
    /// dropped on detach, because a composition visual belongs to the compositor of the tree
    /// it is in and a tile can be moved between trees.
    /// </summary>
    private CompositionDrawingSurface? _surface;

    /// <summary>
    /// The visual the surface is drawn by. It is kept because the size lives here rather than
    /// on the surface: a drawing surface holds pixels and a visual holds where they go.
    /// </summary>
    private CompositionSurfaceVisual? _visual;

    /// <summary>
    /// The imported slots of the current pool, by slot index. Each is imported once and drawn
    /// from many times, which is the whole point of the pool: a per-frame import would be a
    /// per-frame trip through the graphics driver.
    /// </summary>
    private readonly List<ICompositionImportedGpuImage> _slots = [];

    private uint _acquireKey;
    private uint _releaseKey;

    public Control View => this;

    public FrameHandleType Handle => FrameHandleType.D3D11GlobalShared;

    protected override void OnAttachedToVisualTree(VisualTreeAttachmentEventArgs e)
    {
        base.OnAttachedToVisualTree(e);

        var self = ElementComposition.GetElementVisual(this);
        if (self is null)
        {
            return;
        }

        _surface = self.Compositor.CreateDrawingSurface();
        _visual = self.Compositor.CreateSurfaceVisual();
        _visual.Surface = _surface;
        _visual.Size = new Vector(Bounds.Width, Bounds.Height);
        ElementComposition.SetElementChildVisual(this, _visual);
    }

    protected override void OnDetachedFromVisualTree(VisualTreeAttachmentEventArgs e)
    {
        base.OnDetachedFromVisualTree(e);

        ElementComposition.SetElementChildVisual(this, null);
        _surface = null;
        _visual = null;
    }

    protected override Size ArrangeOverride(Size size)
    {
        var arranged = base.ArrangeOverride(size);
        if (_visual is not null)
        {
            _visual.Size = new Vector(arranged.Width, arranged.Height);
        }
        return arranged;
    }

    public async Task<string?> ImportAsync(FramePool pool, CancellationToken cancellation)
    {
        await ReleaseAsync().ConfigureAwait(true);

        var self = ElementComposition.GetElementVisual(this);
        if (self is null || _surface is null)
        {
            return "This tile is not on screen.";
        }

        var interop = await self.Compositor.TryGetCompositionGpuInterop().ConfigureAwait(true);
        if (interop is null)
        {
            return "This window's renderer cannot import a shared texture.";
        }

        var handleType = HandleTypeOf(pool.HandleType);
        if (handleType is null || !interop.SupportedImageHandleTypes.Contains(handleType))
        {
            return "This window's renderer cannot open the kind of shared frame this machine decodes into.";
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

        _acquireKey = pool.ConsumerKey;
        _releaseKey = pool.ProducerKey;
        return null;
    }

    /// <summary>
    /// Draws one lent slot and waits for the compositor to take it.
    ///
    /// The await is the flow control. It completes when the compositor has taken the texture,
    /// which is when the slot is genuinely free, so the release that follows is a statement
    /// rather than a guess - and a tile the compositor is slow to serve stops asking for frames
    /// instead of overwriting one it is still drawing.
    /// </summary>
    public Task DrawAsync(uint slot, CancellationToken cancellation)
    {
        if (_surface is null || slot >= _slots.Count)
        {
            return Task.CompletedTask;
        }

        return _surface.UpdateWithKeyedMutexAsync(_slots[(int)slot], _acquireKey, _releaseKey);
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
    }

    public async ValueTask DisposeAsync() => await ReleaseAsync().ConfigureAwait(true);

    /// <summary>
    /// The compositor's name for a handle type the backend can lend, and null for one this
    /// surface does not import.
    ///
    /// This is where the contract's identifiers meet the toolkit's, and it is a map rather than
    /// an assumption: which handle types a backend imports differs between two graphics
    /// backends on one operating system, which is why the supported list is asked for rather
    /// than derived from the platform.
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
