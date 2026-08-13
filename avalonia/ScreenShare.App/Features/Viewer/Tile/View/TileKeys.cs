using System.Windows.Input;

using Avalonia.Input;

using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;

namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// The keys a tile answers to, as the one table the menu row and the press both read.
///
/// A gesture written into the markup and a switch over <see cref="Key"/> beside it would be two answers to
/// what F does, and the menu would go on printing the older one for as long as nobody compared them.
/// The row takes its gesture from here and so does the press that acts on it.
///
/// Every row of the menu carries a key but one: the three arrangements, the mute, the stats overlay, and the
/// pair that moves the volume.
/// Tone mapping is the exception, because it rebuilds the decode and the tile goes dark for as long as that
/// takes, which is not something a resting pointer should be able to do with one letter.
/// </summary>
public static class TileKeys
{
    /// <summary>Focuses the tile, or gives up focus when it already has it.</summary>
    public static KeyGesture Focus { get; } = new(Key.O);

    /// <summary>Draws the stream in a window of its own, or returns it to the grid.</summary>
    public static KeyGesture PopOut { get; } = new(Key.P);

    /// <summary>Fills a screen with the stream, or gives the screen back.</summary>
    public static KeyGesture Fullscreen { get; } = new(Key.F);

    /// <summary>Silences the decode, or unsilences it at the volume that was chosen.</summary>
    public static KeyGesture Mute { get; } = new(Key.M);

    /// <summary>Draws the figures over the tile, or stops.</summary>
    public static KeyGesture Stats { get; } = new(Key.S);

    /// <summary>
    /// Plays the decode one step louder.
    /// A gesture resolves the numeric keypad's own operator onto this key, so both plus keys land here.
    /// </summary>
    public static KeyGesture Louder { get; } = new(Key.OemPlus);

    /// <summary>Plays it one step quieter.</summary>
    public static KeyGesture Quieter { get; } = new(Key.OemMinus);

    /// <summary>
    /// The same key on a layout that prints + over =, where the character costs a Shift.
    ///
    /// A gesture carries its modifiers, so a shifted press is a different gesture rather than the same one
    /// held differently, and without this entry + would raise the volume on a German keyboard and do nothing
    /// on an American one.
    /// It is not printed anywhere: the menu names the key, and the key is the one <see cref="Louder"/> names.
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
    /// What a press asks of this tile, and null for a key that is none of these.
    ///
    /// The gesture decides, so a held modifier is a different gesture and matches nothing here beyond the one
    /// entry that states its own: Ctrl+F belongs to whatever else claims it and never to a tile.
    /// Whether the tile can answer is the command's own question, asked where the press is acted on.
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
