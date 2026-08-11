using Avalonia.Controls;
using ScreenShare.Api.V1;

namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// The drawing half of a tile: what a lent slot becomes on screen.
///
/// <b>It is per handle type and not per platform.</b> A pool announces what its slots are -
/// a shared texture, a dmabuf descriptor - and which of those a machine can open is a
/// property of the renderer rather than of the operating system, since two graphics backends
/// on one system import different lists. So the surface is chosen from the pool's own
/// statement (<see cref="TileSurfaces.For"/>), and each implementation is the whole of one
/// handle type's import.
///
/// <b>The tile owns the subscription and the surface owns the import.</b> Which slot to draw,
/// when to release it and what to report are the same on every platform and stay in
/// <see cref="StreamTile"/>; how a slot becomes a picture is the only part that differs, and
/// it is all that is behind this interface.
/// </summary>
internal interface ITileSurface : IAsyncDisposable
{
    /// <summary>
    /// The control that draws. Both implementations are the control, and it is named
    /// separately because they derive from different Avalonia bases: one owns a composition
    /// surface and the other is an OpenGL control.
    /// </summary>
    Control View { get; }

    /// <summary>
    /// The handle type this surface imports. A tile reads it to tell a pool it can keep drawing
    /// through the surface it has from one it needs another for, which is a question asked of
    /// the surface rather than remembered beside it.
    /// </summary>
    FrameHandleType Handle { get; }

    /// <summary>
    /// Imports one pool's slots, replacing whatever the previous one left.
    ///
    /// The answer is what the tile shows instead of a picture, and null is a pool this machine
    /// can draw. A renderer that cannot open the handle type would fail per frame with the
    /// same reason, so it is said once, here, beside the tile.
    /// </summary>
    Task<string?> ImportAsync(FramePool pool, CancellationToken cancellation);

    /// <summary>
    /// Draws one lent slot. The task completes when the slot is free again, which is what
    /// makes the release that follows a statement rather than a guess.
    /// </summary>
    Task DrawAsync(uint slot, CancellationToken cancellation);
}

/// <summary>
/// Which surface draws which handle type. It is the one table that pairs the contract's
/// identifiers with the imports this app has, so a handle type nothing here opens is a tile
/// that says so rather than a control that fails per frame.
/// </summary>
internal static class TileSurfaces
{
    /// <summary>
    /// The surface for one handle type, and null for a kind no import here knows about.
    /// </summary>
    public static ITileSurface? For(FrameHandleType type) => type switch
    {
        FrameHandleType.D3D11GlobalShared => new SharedTextureSurface(),
        FrameHandleType.DmabufFd => new DmaBufSurface(),
        _ => null,
    };
}
