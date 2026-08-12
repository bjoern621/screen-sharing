using ScreenShare.App.Backend;
using ScreenShare.App.Features.Viewer.Tile.Model;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A tile drawing a stream that carries more range than the display shows.
///
/// <b>The picture is not obviously wrong, which is the whole difficulty.</b> An HDR stream drawn
/// as it arrives is a picture with the wrong brightness, and a reader with nothing to read blames
/// the stream, the encoder or their own screen. So the tile says which curve it is drawing and
/// what it is doing about it, in both states, and the control that changes it is one press away.
///
/// <b>What the tile reports is what the decode was built with.</b> Tone mapping is an element in
/// the pipeline, so the answer changes by rebuilding the decode, and the tick comes back through
/// the decode's own state. A tile that showed the request instead would claim a conversion on a
/// machine that has nothing to perform it with.
/// </summary>
public sealed class ToneMapTests
{
    private static TileViewModel Tile(IBackend backend)
        => new(TileSource.Relay("desk", "rtsp"), backend, static action => action(), static _ => { });

    /// <summary>One decode's state, as the receive state reports it.</summary>
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

        tile.Apply(Decode(hdr: true));

        Assert.True(tile.IsHdr);
        Assert.False(tile.ToneMapped);
        Assert.True(tile.HasColourNote);
        Assert.Contains("HDR (PQ)", tile.ColourNote);
        Assert.Contains("as it arrives", tile.ColourNote);
    }

    /// <summary>
    /// The badge stays once the conversion is on, because the reader it is written for is the one
    /// comparing two tiles of one stream: a tile that fell silent would leave them unable to tell
    /// which of the two they are looking at.
    /// </summary>
    [Fact]
    public void ATileThatIsRollingTheRangeDownSaysThatToo()
    {
        var tile = Tile(new SeededBackend("linux"));

        tile.Apply(Decode(hdr: true, toneMap: true));

        Assert.True(tile.ToneMapped);
        Assert.True(tile.HasColourNote);
        Assert.Contains("rolled down", tile.ColourNote);
    }

    /// <summary>
    /// A stream whose range this display shows says nothing about colour and offers nothing to
    /// change: a badge over every tile would be noise, and a control that converts nothing is one
    /// press to find out it does nothing.
    /// </summary>
    [Fact]
    public void AStandardRangeStreamCarriesNoBadgeAndNoChoice()
    {
        var tile = Tile(new SeededBackend("linux"));

        tile.Apply(Decode(hdr: false));

        Assert.False(tile.IsHdr);
        Assert.False(tile.HasColourNote);
        Assert.False(tile.CanToneMap);
        Assert.False(tile.ToggleToneMap.CanExecute(null));
        Assert.Contains("range this display shows", tile.ToneMapNote);
    }

    /// <summary>
    /// A machine with nothing to convert with names the element, because that is a thing to go and
    /// install. A greyed control that says nothing teaches nothing, which is the contract every
    /// refused option in this app keeps.
    /// </summary>
    [Fact]
    public void AMachineThatCannotRollTheRangeDownNamesWhatIsMissing()
    {
        var tile = Tile(new SeededBackend("linux"));

        tile.Apply(Decode(hdr: true, canToneMap: false, missing: "vapostproc"));

        Assert.False(tile.CanToneMap);
        Assert.False(tile.ToggleToneMap.CanExecute(null));
        Assert.Contains("vapostproc", tile.ToneMapNote);
    }

    /// <summary>
    /// A platform with no such route at all reads differently, and the difference matters: there
    /// is nothing to install, so a sentence naming an element would send the reader after a
    /// package that would not help.
    /// </summary>
    [Fact]
    public void APlatformWithNoRouteSaysThatRatherThanNamingAnElement()
    {
        var tile = Tile(new SeededBackend("linux"));

        tile.Apply(Decode(hdr: true, canToneMap: false));

        Assert.False(tile.CanToneMap);
        Assert.Contains("platform", tile.ToneMapNote);
    }

    /// <summary>
    /// The press asks for the other answer on the same decode, which is one decode and not two:
    /// the call names the state the decode should be in, and the backend rebuilds it.
    /// </summary>
    [Fact]
    public async Task TheControlAsksForTheOtherAnswerOnTheSameDecode()
    {
        var backend = new SeededBackend("linux") { Hdr = true, Transfer = "smpte2084", CanToneMap = true };
        await backend.StartReceiveAsync("desk", "rtsp");

        var tile = Tile(backend);
        tile.Apply(Decode(hdr: true));
        tile.ToggleToneMap.Execute(null);

        Assert.Single(backend.Decoded);
        var decode = Assert.Single(await backend.ReceivingAsync());
        Assert.True(decode.ToneMap);
    }

    /// <summary>
    /// The tick is the decode's answer and never the request, which is what a machine with no
    /// element for it makes visible: the press is sent, the decode is built without the rung, and
    /// the tile goes on saying the stream is drawn as it arrives.
    ///
    /// It is the shell's half of the rule the backend keeps on the same call. Held the other way
    /// round, a viewer would show a conversion that never happened and would ask for it again on
    /// every pass.
    /// </summary>
    [Fact]
    public async Task AskingForAConversionThisMachineCannotMakeLeavesTheTileSayingSo()
    {
        var backend = new SeededBackend("linux") { Hdr = true, Transfer = "smpte2084", CanToneMap = false };
        await backend.StartReceiveAsync("desk", "rtsp");

        var tile = Tile(backend);
        tile.Apply(Decode(hdr: true, canToneMap: false, missing: "vapostproc"));

        var decode = Assert.Single(await backend.ReceivingAsync());
        tile.Apply(TilePipeline.Of(decode));

        Assert.False(tile.ToneMapped);
        Assert.Contains("as it arrives", tile.ColourNote);
    }

    /// <summary>
    /// The publish's own preview is not offered the choice, because it is not opened by the call
    /// that carries it: the preview belongs to the publish and has neither half of the pair a
    /// decode is keyed by.
    /// </summary>
    [Fact]
    public void ThePublishPreviewIsOfferedNoChoice()
    {
        var tile = new TileViewModel(
            TileSource.Preview("desk"), new SeededBackend("linux"), static action => action(), static _ => { });

        tile.Apply(Decode(hdr: true));

        Assert.False(tile.CanToneMap);
        Assert.False(tile.ToggleToneMap.CanExecute(null));
        Assert.Equal("", tile.ToneMapNote);
    }
}
