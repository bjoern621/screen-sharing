using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Viewer.Tile.Model;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A tile with a decode open and no frame out of it yet is connecting, which the design gives the turning arc
/// (<c>docs/design-language.md</c>, "Status language").
/// The three other dark states are not waits: a pipeline that stated why, a decode nobody opened,
/// and a stream already live.
/// Asserted: which states claim the arc.
/// The arc is an animation and carries no readable state.
/// </summary>
public sealed class TileConnectingTests
{
    private static readonly Action<Action> Inline = action => action();

    private static TileViewModel Tile()
        => new(TileSource.Relay("desk", "rtsp"), new SeededBackend("linux"), Inline, static _ => { });

    private static TilePipeline Decode(bool live, Text? failure = null)
        => new(live, HasAudio: false, Volume: 1, Muted: false, Failure: failure);

    [Fact]
    public void ADecodeWithNoFrameOutOfItYetIsConnecting()
    {
        var tile = Tile();

        tile.Apply(Decode(live: false), sample: null);

        Assert.True(tile.HasNotice);
        Assert.False(tile.NoticeIsFailure);
        Assert.True(tile.IsConnecting);
    }

    [Fact]
    public void AStreamAlreadyLiveIsNotConnecting()
    {
        var tile = Tile();

        tile.Apply(Decode(live: true), sample: null);

        Assert.False(tile.HasNotice);
        Assert.False(tile.IsConnecting);
    }

    /// <summary>
    /// A pipeline carrying its own reason is a failure, and a failure is carried in words.
    /// An arc over it would promise a picture nothing is bringing.
    /// </summary>
    [Fact]
    public void APipelineThatStatedWhyIsNotConnecting()
    {
        var tile = Tile();

        tile.Apply(Decode(live: false, new Text { Code = TextCode.StreamLeftTheRelay }), sample: null);

        Assert.True(tile.NoticeIsFailure);
        Assert.False(tile.IsConnecting);
    }

    /// <summary>
    /// A decode nobody opened waits on a press rather than on an answer, so it names what it needs and turns nothing.
    /// </summary>
    [Fact]
    public void ADecodeNobodyOpenedIsNotConnecting()
    {
        var tile = Tile();

        tile.Apply(pipeline: null, sample: null);

        Assert.True(tile.HasNotice);
        Assert.False(tile.IsConnecting);
    }
}
