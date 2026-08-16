using Avalonia;
using Avalonia.Controls;
using Avalonia.Platform;
using Avalonia.Rendering.Composition;
using ScreenShare.Api.V1;

namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// Import path for a handle the compositor takes itself: a shared texture named to Avalonia's device and drawn as
/// a composition visual.
///
/// No pixel is read.
/// The decoder writes the texture, the frame channel carries its name, and the name goes to the compositor:
/// nothing in a message, nothing in system memory, nothing copied here (<c>docs/viewer-architecture.md</c>, "The
/// frame channel").
///
/// Deliberately not a <see cref="NativeControlHost"/>.
/// Handing GStreamer a window handle is easier and wrong: a native child window paints over every Avalonia
/// control, burying the name, the badge and the stats panel that belong above the video.
/// A composition visual sits among the others (<c>avalonia/README.md</c>).
///
/// A frame is a slot on loan, returned once the compositor has taken it, which
/// <see cref="CompositionDrawingSurface.UpdateWithKeyedMutexAsync"/> resolves on.
/// A slow tile costs frames the backend drops, and never a torn picture or a stalled pipeline.
/// </summary>
internal sealed class SharedTextureSurface : Control, ITileSurface
{
    /// <summary>
    /// Drawing surface, built on attach and released on detach.
    /// A composition object belongs to the compositor of its tree, and a tile is moved between trees.
    /// </summary>
    private CompositionDrawingSurface? _surface;

    /// <summary>
    /// Visual carrying the surface.
    /// Kept because size lives on it: pixels are the surface's, placement the visual's.
    /// </summary>
    private CompositionSurfaceVisual? _visual;

    /// <summary>
    /// Imports of the live pool, by slot index.
    /// Each is imported once and drawn from repeatedly; importing per frame would cross the graphics driver per
    /// frame.
    /// </summary>
    private readonly List<ICompositionImportedGpuImage> _slots = [];

    /// <summary>
    /// Keyed-mutex keys, taken crossed: this side acquires on the pool's consumer key and releases on its producer
    /// key, so neither end hardcodes a number the other picked.
    /// Zero on a handle type synchronized some other way.
    /// </summary>
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

        // Queried of the renderer rather than derived from the platform, and refused outright: copying through
        // system memory is the gigabyte-a-second fallback the frame channel exists to avoid.
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
    /// Draws one lent slot, resolving when the compositor has taken it.
    /// Flow control lives in that await: it marks the slot genuinely free, so the release behind it reports rather
    /// than assumes, and a tile the compositor serves slowly stops asking for frames instead of overwriting one
    /// still being drawn.
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
    /// Discards the imports of a pool no longer current.
    /// Awaited, not fired off: the release runs on the render thread while the backend frees the texture as the
    /// call ends, and one still in flight would land on memory that is gone.
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
    /// Toolkit's name for a handle type the backend lends, null for one this surface leaves alone.
    /// A table rather than a guess: two graphics backends on one operating system import different sets, hence the
    /// supported list being queried instead of inferred from the platform.
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
