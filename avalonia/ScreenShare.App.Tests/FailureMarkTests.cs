using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Viewer.Tile.Model;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Which of a viewer's sentences is a failure, red being spent on those alone
/// (<c>docs/design-language.md</c>, "Palette").
/// A backend that answered nothing and a relay carrying nothing both leave the screen empty,
/// and only the first is broken.
/// Asserted: which sentences the screen marks. What the mark looks like is the palette's.
/// </summary>
public sealed class FailureMarkTests
{
    private static readonly Action<Action> Inline = action => action();

    private static TileViewModel Tile()
        => new(TileSource.Relay("desk", "rtsp"), new SeededBackend("linux"), Inline, static _ => { });

    private static TilePipeline Decode(bool live, Text? failure = null)
        => new(live, HasAudio: false, Volume: 1, Muted: false, Failure: failure);

    /// <summary>Backend's own sentence, standing where the relay's states would be.</summary>
    [Fact]
    public void TheNoticeAboutAnAbsentBackendIsAFailure()
    {
        var backend = new DeferredBackend { IsAbsent = true };
        var session = new Session(backend, Inline);
        var viewer = Flows.Viewer(backend, session);

        session.Start();
        session.Stop();
        viewer.Apply();

        Assert.Equal(session.Unavailable, viewer.Notice);
        Assert.True(viewer.NoticeIsFailure);
    }

    /// <summary>A relay with nothing on it answered, so the sentence standing in for the list is news.</summary>
    [Fact]
    public void TheNoticeAboutTheRelayIsNot()
    {
        var backend = new DeferredBackend();
        var session = new Session(backend, Inline);
        var viewer = Flows.Viewer(backend, session);

        session.Start();
        session.Stop();
        viewer.Apply();

        Assert.True(viewer.HasNotice);
        Assert.False(viewer.NoticeIsFailure);
    }

    [Fact]
    public void ATileSayingWhyNothingIsArrivingIsAFailure()
    {
        var tile = Tile();

        tile.Apply(Decode(live: false, new Text { Code = TextCode.StreamLeftTheRelay }), sample: null);

        Assert.True(tile.NoticeIsFailure);
    }

    /// <summary>What the control itself could not draw, a driver or a handle type nothing else here names.</summary>
    [Fact]
    public void ATileReportingWhatItCouldNotDrawIsAFailure()
    {
        var tile = Tile();

        tile.Report(new TileReport(0, 0, 0, 0, "This renderer cannot import a dmabuf handle."));
        tile.Apply(Decode(live: false), sample: null);

        Assert.True(tile.NoticeIsFailure);
    }

    /// <summary>A decode still opening is on its way, and a red sentence over it would say it stopped.</summary>
    [Fact]
    public void ATileStillConnectingIsNot()
    {
        var tile = Tile();

        tile.Apply(Decode(live: false), sample: null);

        Assert.Equal("Connecting.", tile.Notice);
        Assert.False(tile.NoticeIsFailure);
    }

    /// <summary>A stream nobody asked for is idle, which is a state and not a fault.</summary>
    [Fact]
    public void ATileNobodyOpenedADecodeForIsNot()
    {
        var tile = Tile();

        tile.Apply(pipeline: null, sample: null);

        Assert.True(tile.HasNotice);
        Assert.False(tile.NoticeIsFailure);
    }

    /// <summary>A tile drawing frames says nothing, so there is nothing to mark.</summary>
    [Fact]
    public void ATileDrawingAPictureMarksNothing()
    {
        var tile = Tile();

        tile.Apply(Decode(live: true), sample: null);

        Assert.False(tile.HasNotice);
        Assert.False(tile.NoticeIsFailure);
    }
}
