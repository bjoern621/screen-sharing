using ScreenShare.App.Backend;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Whether a screen says its first answer is still out.
/// A window that has asked and not been answered draws the same turning arc a pressed control wears,
/// so an empty screen on launch reads as one that is working rather than one that is broken.
///
/// Separate from the redial the arc also marks (<see cref="RedialIndicatorTests"/>):
/// a backend that refused or could not be reached says so in words, and words are what carries a failure.
/// Asserted: which screens claim it.
/// The arc is an animation and carries no readable state.
/// </summary>
public sealed class FirstReadIndicatorTests
{
    /// <summary>Reads every state once and stops before the reconnect delay, as the redial suite does.</summary>
    private static void Load(Session session)
    {
        session.Start();
        session.Stop();
    }

    private static Session Fresh(IBackend backend) => new(backend, action => action());

    /// <summary>
    /// The state every window opens in: asked, unanswered, and nothing wrong yet.
    /// </summary>
    [Fact]
    public void ASessionThatHasNotBeenAnsweredIsReading()
    {
        var session = Fresh(new DeferredBackend());

        Assert.False(session.IsLoaded);
        Assert.Equal("", session.Unavailable);
        Assert.True(session.IsReading);
    }

    [Fact]
    public void TheFirstAnswerStopsTheIndicator()
    {
        var backend = new DeferredBackend();
        var session = Fresh(backend);

        Load(session);

        Assert.True(session.IsLoaded);
        Assert.False(session.IsReading);
    }

    /// <summary>
    /// An unreachable backend is a failure the sentence carries, so the read is over rather than outstanding.
    /// The redial arc is what turns there, off the state that suite asserts.
    /// </summary>
    [Fact]
    public void AnUnreachableBackendIsNotReading()
    {
        var backend = new DeferredBackend { IsAbsent = true };
        var session = Fresh(backend);

        Load(session);

        Assert.NotEqual("", session.Unavailable);
        Assert.False(session.IsReading);
    }

    /// <summary>
    /// Window opens on the viewer, so this is the screen a launch is watched on.
    /// The rail already says it is reading the relay, and the arc is what makes that sentence a wait.
    /// </summary>
    [Fact]
    public void TheViewerSaysItBesideTheSentenceAboutTheRelay()
    {
        var backend = new DeferredBackend();
        var session = Fresh(backend);
        var viewer = Flows.Viewer(backend, session);

        viewer.Apply();

        Assert.True(viewer.HasNotice);
        Assert.False(viewer.IsDialling);
        Assert.True(viewer.IsReading);
    }

    /// <summary>
    /// The member card reads the same session, so it waits and stops waiting with the screen holding it
    /// rather than off a second reading of its own.
    /// </summary>
    [Fact]
    public void TheMemberCardWaitsWithTheScreenHoldingIt()
    {
        var backend = new DeferredBackend();
        var session = Fresh(backend);
        var viewer = Flows.Viewer(backend, session);

        viewer.Apply();
        Assert.True(viewer.Members.IsReading);

        Load(session);
        viewer.Apply();

        Assert.False(viewer.Members.IsReading);
    }

    /// <summary>
    /// A wizard with no form yet draws no step, which is the blank screen the arc is spent on.
    /// </summary>
    [Fact]
    public void TheWizardSaysItWhileTheFirstFormIsOut()
    {
        var backend = new DeferredBackend();
        var session = Fresh(backend);
        var flow = Flows.Setup(backend, session);

        flow.Apply();

        Assert.False(flow.IsUnavailable);
        Assert.True(flow.IsReading);
    }

    /// <summary>
    /// Once the wizard has a form to draw, the arc has nothing left to say and the steps carry the screen.
    /// </summary>
    [Fact]
    public async Task AResolvedFormStopsTheWizardIndicator()
    {
        var backend = new DeferredBackend();
        var session = Fresh(backend);
        var flow = Flows.Setup(backend, session);

        Load(session);
        await backend.AnswerAsync(0);
        flow.Apply();

        Assert.False(flow.IsReading);
    }
}
