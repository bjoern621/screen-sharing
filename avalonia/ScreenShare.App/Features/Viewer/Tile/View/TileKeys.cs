using Avalonia.Input;

using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// The keys a tile answers to, as the one table the menu row and the press both read.
///
/// A gesture written into the markup and a switch over <see cref="Key"/> beside it would be two answers to
/// what F does, and the menu would go on printing the older one for as long as nobody compared them.
/// The row takes its gesture from here and so does the press that acts on it.
///
/// Only the three arrangements have keys.
/// Mute is a call the backend can refuse and the stats overlay is a diagnostic, and neither is worth a letter
/// a resting pointer can hit.
/// </summary>
public static class TileKeys
{
    /// <summary>Focuses the tile, or gives up focus when it already has it.</summary>
    public static KeyGesture Focus { get; } = new(Key.O);

    /// <summary>Draws the stream in a window of its own, or returns it to the grid.</summary>
    public static KeyGesture PopOut { get; } = new(Key.P);

    /// <summary>Fills a screen with the stream, or gives the screen back.</summary>
    public static KeyGesture Fullscreen { get; } = new(Key.F);

    private static readonly (KeyGesture Gesture, Func<TileViewModel, DelegateCommand> Command)[] Table =
    [
        (Focus, tile => tile.ToggleFocus),
        (PopOut, tile => tile.TogglePopOut),
        (Fullscreen, tile => tile.ToggleFullscreen),
    ];

    /// <summary>
    /// What a press asks of this tile, and null for a key that is none of these.
    ///
    /// The gesture decides, so a held modifier is a different gesture and matches nothing here: Ctrl+F
    /// belongs to whatever else claims it and never to a tile.
    /// </summary>
    public static DelegateCommand? Command(TileViewModel tile, KeyEventArgs press)
    {
        Assert.NotNull(tile, "a key press names the tile it acts on");
        Assert.NotNull(press, "a key press is read off the event that carried it");

        foreach (var (gesture, command) in Table)
        {
            if (gesture.Matches(press))
            {
                return command(tile);
            }
        }

        return null;
    }
}
