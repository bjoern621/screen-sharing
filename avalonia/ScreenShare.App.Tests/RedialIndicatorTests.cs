using ScreenShare.App.Backend;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Whether a screen says the window is still dialling the backend.
/// The absent-backend sentence does not move,
/// so a window between two attempts and one that stopped trying draw the same thing without it.
/// Asserted: which screens claim it, not how it looks.
/// The arc is an animation and carries no readable state.
/// </summary>
public sealed class RedialIndicatorTests
{
    /// <summary>
    /// Reads every state once and stops before the reconnect delay, so nothing is left dialling behind the assertions.
    /// </summary>
    private static void Load(Session session)
    {
        session.Start();
        session.Stop();
    }

    [Fact]
    public void TheBannerSaysTheWindowIsStillDiallingBesideTheSentenceSayingWhy()
    {
        var backend = new DeferredBackend { IsAbsent = true };
        var session = new Session(backend, action => action());
        var flow = Flows.Setup(backend, session);

        Load(session);
        flow.Apply();

        Assert.True(flow.IsUnavailable);
        Assert.True(flow.IsDialling);
    }

    /// <summary>
    /// Rail draws on every step, so the preset card's refusal is the sentence a reader on setup sees.
    /// The refusal alone says nothing about a next attempt.
    /// </summary>
    [Fact]
    public void ThePresetCardSaysItUnderTheRefusalItsOwnReadCameBackWith()
    {
        var backend = new DeferredBackend { IsAbsent = true };
        var session = new Session(backend, action => action());
        var flow = Flows.Setup(backend, session);

        Load(session);
        flow.Apply();

        Assert.True(flow.Rail.Presets.HasRefusal);
        Assert.True(flow.Rail.Presets.IsDialling);
    }

    /// <summary>
    /// A refusal the backend served is a failure with the socket up,
    /// so no attempt is coming and the button is the whole of what is offered.
    /// </summary>
    [Fact]
    public void ARefusalTheBackendServedIsNotDialling()
    {
        var backend = new DeferredBackend();
        var flow = Flows.Setup(backend);

        backend.Fail(0, "The settings could not be read.");

        Assert.True(flow.IsUnavailable);
        Assert.False(flow.IsDialling);
    }

    /// <summary>
    /// Window opens on the viewer, so a shell launched before its backend draws this screen and nothing else.
    /// </summary>
    [Fact]
    public void TheViewerSaysItUnderTheNoticeAboutTheBackend()
    {
        var backend = new DeferredBackend { IsAbsent = true };
        var session = new Session(backend, action => action());
        var viewer = Flows.Viewer(backend, session);

        Load(session);
        viewer.Apply();

        Assert.Equal(session.Unavailable, viewer.Notice);
        Assert.True(viewer.IsDialling);
    }

    /// <summary>
    /// A relay notice came through a backend that is up.
    /// </summary>
    [Fact]
    public void ANoticeAboutTheRelayIsNotDialling()
    {
        var backend = new DeferredBackend();
        var session = new Session(backend, action => action());
        var viewer = Flows.Viewer(backend, session);

        Load(session);
        viewer.Apply();

        Assert.True(viewer.HasNotice);
        Assert.False(viewer.IsDialling);
    }

    /// <summary>
    /// The state a window holds for the rest of its life once it reached its backend,
    /// and the one a stale flag would leave an arc turning in.
    /// </summary>
    [Fact]
    public void ABackendThatCameBackStopsTheIndicator()
    {
        var backend = new DeferredBackend { IsAbsent = true };
        var session = new Session(backend, action => action());
        var viewer = Flows.Viewer(backend, session);

        Load(session);
        viewer.Apply();
        Assert.True(viewer.IsDialling);

        backend.IsAbsent = false;
        Load(session);
        viewer.Apply();

        Assert.Equal("", session.Unavailable);
        Assert.False(viewer.IsDialling);
    }
}
