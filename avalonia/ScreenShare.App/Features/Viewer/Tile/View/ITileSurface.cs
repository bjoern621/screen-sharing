using Avalonia.Controls;
using ScreenShare.Api.V1;

namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// A tile's drawing half: one lent slot turned into pixels on screen.
///
/// Keyed on handle type, never on platform.
/// A pool states what its slots are, a shared texture or a dmabuf descriptor, and opening one is a property of
/// the renderer rather than of the operating system, since two graphics backends on a machine take different
/// lists (<see cref="TileSurfaces.For"/>).
/// An implementation here is one handle type's import, entire.
///
/// Slot choice, release and reporting do not vary by machine and stay in <see cref="StreamTile"/>.
/// </summary>
internal interface ITileSurface : IAsyncDisposable
{
    /// <summary>
    /// Control doing the drawing.
    /// Exposed rather than inherited: one implementation owns a composition surface and the other is an
    /// OpenGL control, so they share no Avalonia base.
    /// </summary>
    Control View { get; }

    /// <summary>
    /// Handle type imported here.
    /// Consulted when a pool lands, to separate a surface that carries on from one that has to be replaced.
    /// </summary>
    FrameHandleType Handle { get; }

    /// <summary>
    /// Takes on one pool's slots and discards the previous pool's.
    /// Answers with the sentence a tile shows in place of a picture, null where the pool is drawable here.
    /// A renderer that cannot open this handle type would repeat that verdict per frame, so it is stated once.
    /// </summary>
    Task<string?> ImportAsync(FramePool pool, CancellationToken cancellation);

    /// <summary>
    /// Draws a lent slot, resolving once that slot is free again.
    /// The release behind it therefore reports rather than assumes.
    /// </summary>
    Task DrawAsync(uint slot, CancellationToken cancellation);
}

/// <summary>
/// Handle type to import, in one table.
/// A type absent from it is a tile carrying a sentence, not a control failing once per frame.
/// </summary>
internal static class TileSurfaces
{
    /// <summary>Surface for a handle type, null where nothing here imports it.</summary>
    public static ITileSurface? For(FrameHandleType type) => type switch
    {
        FrameHandleType.D3D11GlobalShared => new SharedTextureSurface(),
        FrameHandleType.DmabufFd => new DmaBufSurface(),
        _ => null,
    };
}
