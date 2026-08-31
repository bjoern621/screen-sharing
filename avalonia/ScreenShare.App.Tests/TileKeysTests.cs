using Avalonia.Input;

using ScreenShare.App.Backend;
using ScreenShare.App.Features.Viewer.Tile.Model;
using ScreenShare.App.Features.Viewer.Tile.View;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// One table serves the press and the menu row that prints the gesture,
/// so neither can name a key the other does not run.
/// A volume key names a level rather than a change:
/// the target is computed from what the decode plays at and read back off the decode's state.
/// </summary>
public sealed class TileKeysTests
{
    private static TileViewModel Tile(IBackend backend)
        => new(TileSource.Relay("desk", "rtsp"), backend, static action => action(), static _ => { });

    private static KeyEventArgs Press(Key key, KeyModifiers modifiers = KeyModifiers.None)
        => new() { Key = key, KeyModifiers = modifiers };

    /// <summary>Renders the tile against the decode the fixture is running, as its screen does.</summary>
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

    [Fact]
    public void AKeyNoRowNamesReachesNothing()
    {
        var tile = Tile(new SeededBackend("linux"));

        Assert.Null(TileKeys.Command(tile, Press(Key.Q)));
    }

    /// <summary>Ctrl+F belongs to whatever else claims it, never to a tile.</summary>
    [Fact]
    public void AHeldModifierIsADifferentKey()
    {
        var tile = Tile(new SeededBackend("linux"));

        Assert.Null(TileKeys.Command(tile, Press(Key.F, KeyModifiers.Control)));
    }

    /// <summary>The keypad operator and the Shift+= one are one request.</summary>
    [Fact]
    public void EveryPlusKeyRaisesTheVolume()
    {
        var tile = Tile(new SeededBackend("linux"));

        Assert.Same(tile.Louder, TileKeys.Command(tile, Press(Key.Add)));
        Assert.Same(tile.Louder, TileKeys.Command(tile, Press(Key.OemPlus, KeyModifiers.Shift)));
        Assert.Same(tile.Quieter, TileKeys.Command(tile, Press(Key.Subtract)));
    }

    /// <summary>The tile draws what came back, so two presses are two steps and not one reported twice.</summary>
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
    /// A press at the top asks for the level already in force,
    /// so holding the key is a run of calls changing nothing.
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
    /// A key is refused wherever the row it names is greyed,
    /// and the card leaves the press for whatever else wants it.
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
