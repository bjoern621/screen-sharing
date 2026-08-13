using ScreenShare.App.Backend;
using ScreenShare.App.Features.Viewer.Tile.Model;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A stream carrying more range than the display shows draws at the wrong brightness, which a reader blames
/// on the stream or the screen, so the tile states the curve and what it is doing about it in both
/// directions.
/// What it states is what the decode was built with: tone mapping is an element in the pipeline, so the tick
/// comes back through the decode's own state rather than off the request.
/// </summary>
public sealed class ToneMapTests
{
    private static TileViewModel Tile(IBackend backend)
        => new(TileSource.Relay("desk", "rtsp"), backend, static action => action(), static _ => { });

    private static TilePipeline Decode(
        bool hdr, bool toneMap = false, bool canToneMap = true, string missing = "", string transfer = "smpte2084")
        => new(
            Live: true, Chain: "gl", RenderMemory: "memory:GLMemory", Decoder: "avdec_h265", Hardware: false,
            HasAudio: false, Volume: 1, Muted: false,
            Transfer: hdr ? transfer : "bt709", Hdr: hdr,
            ToneMap: toneMap, CanToneMap: canToneMap, ToneMapMissing: missing);

    [Fact]
    public void AnHdrStreamDrawnAsItArrivesSaysWhichCurveAndThatNothingIsConverting()
    {
        var tile = Tile(new SeededBackend("linux"));

        tile.Apply(Decode(hdr: true), sample: null);

        Assert.True(tile.IsHdr);
        Assert.False(tile.ToneMapped);
        Assert.True(tile.HasColourNote);
        Assert.Contains("HDR (PQ)", tile.ColourNote);
        Assert.Contains("as it arrives", tile.ColourNote);
    }

    /// <summary>
    /// Two tiles of one stream are told apart by the badge, so a converting tile that fell silent would
    /// leave them alike.
    /// </summary>
    [Fact]
    public void ATileThatIsRollingTheRangeDownSaysThatToo()
    {
        var tile = Tile(new SeededBackend("linux"));

        tile.Apply(Decode(hdr: true, toneMap: true), sample: null);

        Assert.True(tile.ToneMapped);
        Assert.True(tile.HasColourNote);
        Assert.Contains("rolled down", tile.ColourNote);
    }

    /// <summary>
    /// A badge over every tile is noise, and a control that converts nothing costs a press to learn it does
    /// nothing.
    /// </summary>
    [Fact]
    public void AStandardRangeStreamCarriesNoBadgeAndNoChoice()
    {
        var tile = Tile(new SeededBackend("linux"));

        tile.Apply(Decode(hdr: false), sample: null);

        Assert.False(tile.IsHdr);
        Assert.False(tile.HasColourNote);
        Assert.False(tile.CanToneMap);
        Assert.False(tile.ToggleToneMap.CanExecute(null));
        Assert.Contains("range this display shows", tile.ToneMapNote);
    }

    /// <summary>
    /// The missing element is a thing to go and install, and every refused option in this app names its
    /// reason.
    /// </summary>
    [Fact]
    public void AMachineThatCannotRollTheRangeDownNamesWhatIsMissing()
    {
        var tile = Tile(new SeededBackend("linux"));

        tile.Apply(Decode(hdr: true, canToneMap: false, missing: "vapostproc"), sample: null);

        Assert.False(tile.CanToneMap);
        Assert.False(tile.ToggleToneMap.CanExecute(null));
        Assert.Contains("vapostproc", tile.ToneMapNote);
    }

    /// <summary>
    /// Nothing is installable here, so naming an element would send the reader after a package that cannot
    /// help.
    /// </summary>
    [Fact]
    public void APlatformWithNoRouteSaysThatRatherThanNamingAnElement()
    {
        var tile = Tile(new SeededBackend("linux"));

        tile.Apply(Decode(hdr: true, canToneMap: false), sample: null);

        Assert.False(tile.CanToneMap);
        Assert.Contains("platform", tile.ToneMapNote);
    }

    /// <summary>
    /// The call names the state the decode should be in, so the backend rebuilds one decode rather than
    /// opening a second.
    /// </summary>
    [Fact]
    public async Task TheControlAsksForTheOtherAnswerOnTheSameDecode()
    {
        var backend = new SeededBackend("linux") { Hdr = true, Transfer = "smpte2084", CanToneMap = true };
        await backend.StartReceiveAsync("desk", "rtsp");

        var tile = Tile(backend);
        tile.Apply(Decode(hdr: true), sample: null);
        tile.ToggleToneMap.Execute(null);

        Assert.Single(backend.Decoded);
        var decode = Assert.Single(await backend.ReceivingAsync());
        Assert.True(decode.ToneMap);
    }

    /// <summary>
    /// The press is sent, the decode is built without the rung, and the tile goes on saying so.
    /// Read off the request instead, a viewer would show a conversion that never happened and ask for it
    /// again on every pass.
    /// </summary>
    [Fact]
    public async Task AskingForAConversionThisMachineCannotMakeLeavesTheTileSayingSo()
    {
        var backend = new SeededBackend("linux") { Hdr = true, Transfer = "smpte2084", CanToneMap = false };
        await backend.StartReceiveAsync("desk", "rtsp");

        var tile = Tile(backend);
        tile.Apply(Decode(hdr: true, canToneMap: false, missing: "vapostproc"), sample: null);

        var decode = Assert.Single(await backend.ReceivingAsync());
        tile.Apply(TilePipeline.Of(decode), sample: null);

        Assert.False(tile.ToneMapped);
        Assert.Contains("as it arrives", tile.ColourNote);
    }

    /// <summary>
    /// The preview belongs to the publish and carries neither half of the stream and leg pair a decode is
    /// keyed by.
    /// </summary>
    [Fact]
    public void ThePublishPreviewIsOfferedNoChoice()
    {
        var tile = new TileViewModel(
            TileSource.Preview("desk"), new SeededBackend("linux"), static action => action(), static _ => { });

        tile.Apply(Decode(hdr: true), sample: null);

        Assert.False(tile.CanToneMap);
        Assert.False(tile.ToggleToneMap.CanExecute(null));
        Assert.Equal("", tile.ToneMapNote);
    }
}
