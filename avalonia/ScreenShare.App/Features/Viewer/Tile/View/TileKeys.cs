using System.Windows.Input;

using Avalonia.Input;

using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;

namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// Keys a tile answers, as the one table a menu row and a press both read.
///
/// A gesture in the markup with a switch over <see cref="Key"/> beside it would be two answers to what F does,
/// and the menu would keep printing the stale one until somebody compared them.
///
/// Each key names a state, so pressing it twice is a round trip.
/// The tone-map row carries none: it rebuilds the decode and blanks the tile for as long as that takes,
/// too much for one letter under a resting pointer.
/// </summary>
public static class TileKeys
{
    public static KeyGesture Focus { get; } = new(Key.O);

    public static KeyGesture PopOut { get; } = new(Key.P);

    public static KeyGesture Fullscreen { get; } = new(Key.F);

    public static KeyGesture Mute { get; } = new(Key.M);

    public static KeyGesture Stats { get; } = new(Key.S);

    /// <summary>
    /// One step up.
    /// The gesture folds the numeric keypad's own operator onto this key, so either plus arrives here.
    /// </summary>
    public static KeyGesture Louder { get; } = new(Key.OemPlus);

    public static KeyGesture Quieter { get; } = new(Key.OemMinus);

    /// <summary>
    /// Same key where the layout prints + above =, so the character costs a Shift.
    /// Modifiers are part of a gesture, so a shifted press is a different gesture:
    /// without this entry + works on a German keyboard and does nothing on an American one.
    /// Printed nowhere, the menu showing the key <see cref="Louder"/> names.
    /// </summary>
    private static readonly KeyGesture ShiftedLouder = new(Key.OemPlus, KeyModifiers.Shift);

    private static readonly (KeyGesture Gesture, Func<TileViewModel, ICommand> Command)[] Table =
    [
        (Focus, tile => tile.ToggleFocus),
        (PopOut, tile => tile.TogglePopOut),
        (Fullscreen, tile => tile.ToggleFullscreen),
        (Mute, tile => tile.ToggleMute),
        (Stats, tile => tile.ToggleStats),
        (Louder, tile => tile.Louder),
        (ShiftedLouder, tile => tile.Louder),
        (Quieter, tile => tile.Quieter),
    ];

    /// <summary>
    /// Command a press asks for, null for a key none of these name.
    /// Matching is by gesture, so a held modifier misses every entry but one carrying that modifier:
    /// Ctrl+F goes to whoever else claims it and never to a tile.
    /// Whether the tile can answer is the command's question, asked where the press is acted on.
    /// </summary>
    public static ICommand? Command(TileViewModel tile, KeyEventArgs press)
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
