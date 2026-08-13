using Avalonia.Input;

using ScreenShare.App.Backend;
using ScreenShare.App.Features.Viewer.Tile.Model;
using ScreenShare.App.Features.Viewer.Tile.View;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The keys a tile answers to.
///
/// <b>One table serves the press and the menu row that prints the key.</b> A gesture in the markup and a
/// switch beside it would be two answers to what a letter does, and the menu would go on printing the older
/// one for as long as nobody compared them - so what is locked here is that each key reaches the command its
/// row runs.
///
/// <b>A volume key names a level rather than a change.</b> It computes its target from what the decode is
/// playing at, sends that, and reads the answer back off the decode's state, which is what makes a press at
/// the end of the range a call that changes nothing rather than one that runs off it.
/// </summary>
public sealed class TileKeysTests
{
    private static TileViewModel Tile(IBackend backend)
        => new(TileSource.Relay("desk", "rtsp"), backend, static action => action(), static _ => { });

    private static KeyEventArgs Press(Key key, KeyModifiers modifiers = KeyModifiers.None)
        => new() { Key = key, KeyModifiers = modifiers };

    /// <summary>Renders the tile against the decode the fixture is running, as the screen holding it does.</summary>
    private static async Task ApplyAsync(TileViewModel tile, SeededBackend backend)
        => tile.Apply(TilePipeline.Of(Assert.Single(await backend.ReceivingAsync())), sample: null);

    [Fact]
    public void EachKeyRunsTheRowThatPrintsIt()
    {
        var tile = Tile(new SeededBackend("linux"));

        Assert.Same(tile.ToggleFocus, TileKeys.Command(tile, Press(Key.O)));
        Assert.Same(tile.TogglePopOut, TileKeys.Command(tile, Press(Key.P)));
        Assert.Same(tile.ToggleFullscreen, TileKeys.Command(tile, Press(Key.F)));
        Assert.Same(tile.ToggleMute, TileKeys.Command(tile, Press(Key.M)));
        Assert.Same(tile.ToggleStats, TileKeys.Command(tile, Press(Key.S)));
        Assert.Same(tile.Louder, TileKeys.Command(tile, Press(Key.OemPlus)));
        Assert.Same(tile.Quieter, TileKeys.Command(tile, Press(Key.OemMinus)));
    }

    /// <summary>A key that names none of the rows is nobody's, so the press travels on.</summary>
    [Fact]
    public void AKeyNoRowNamesReachesNothing()
    {
        var tile = Tile(new SeededBackend("linux"));

        Assert.Null(TileKeys.Command(tile, Press(Key.Q)));
    }

    /// <summary>
    /// A held modifier is a different gesture: Ctrl+F belongs to whatever else claims it, never to a tile.
    /// </summary>
    [Fact]
    public void AHeldModifierIsADifferentKey()
    {
        var tile = Tile(new SeededBackend("linux"));

        Assert.Null(TileKeys.Command(tile, Press(Key.F, KeyModifiers.Control)));
    }

    /// <summary>
    /// Both plus keys raise the volume: the numeric keypad's own operator, and the one a layout puts over =
    /// and charges a Shift for.
    /// A reader pressing + is asking for the same thing on either.
    /// </summary>
    [Fact]
    public void EveryPlusKeyRaisesTheVolume()
    {
        var tile = Tile(new SeededBackend("linux"));

        Assert.Same(tile.Louder, TileKeys.Command(tile, Press(Key.Add)));
        Assert.Same(tile.Louder, TileKeys.Command(tile, Press(Key.OemPlus, KeyModifiers.Shift)));
        Assert.Same(tile.Quieter, TileKeys.Command(tile, Press(Key.Subtract)));
    }

    /// <summary>
    /// A press sends the level it wants and the tile draws what came back, so two presses are two steps
    /// rather than one step reported twice.
    /// </summary>
    [Fact]
    public async Task AVolumeKeyMovesTheDecodeOneStep()
    {
        var backend = new SeededBackend("linux") { HasAudio = true };
        await backend.StartReceiveAsync("desk", "rtsp");

        var tile = Tile(backend);
        await ApplyAsync(tile, backend);

        tile.Quieter.Execute(null);
        await ApplyAsync(tile, backend);
        Assert.Equal(0.95, tile.Volume, 3);

        tile.Quieter.Execute(null);
        await ApplyAsync(tile, backend);
        Assert.Equal(0.90, tile.Volume, 3);

        tile.Louder.Execute(null);
        await ApplyAsync(tile, backend);
        Assert.Equal(0.95, tile.Volume, 3);
    }

    /// <summary>
    /// A press at the top of the range asks for the level it is already at, which the decode is already in:
    /// the key names a state, so holding it there is a run of calls that change nothing rather than a value
    /// climbing past one.
    /// </summary>
    [Fact]
    public async Task AVolumeKeyStopsAtTheEndOfTheRange()
    {
        var backend = new SeededBackend("linux") { HasAudio = true };
        await backend.StartReceiveAsync("desk", "rtsp");

        var tile = Tile(backend);
        await ApplyAsync(tile, backend);

        tile.Louder.Execute(null);
        tile.Louder.Execute(null);
        await ApplyAsync(tile, backend);

        Assert.Equal(1, tile.Volume, 3);
    }

    /// <summary>
    /// A stream with no sound track has nothing to be loud, so the keys that move it are refused where the
    /// rows they name are greyed.
    /// The press is left unhandled by the card, which is what keeps a key this tile cannot answer available
    /// to whatever else wants it.
    /// </summary>
    [Fact]
    public async Task AStreamWithNoSoundTrackRefusesTheAudioKeys()
    {
        var backend = new SeededBackend("linux");
        await backend.StartReceiveAsync("desk", "rtsp");

        var tile = Tile(backend);
        await ApplyAsync(tile, backend);

        Assert.False(tile.HasAudio);
        Assert.False(tile.Louder.CanExecute(null));
        Assert.False(tile.Quieter.CanExecute(null));
        Assert.False(tile.ToggleMute.CanExecute(null));
    }
}
