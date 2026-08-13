using ScreenShare.App.Backend;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The setup flow against a backend that holds its answers instead of answering out of a dictionary.
/// <see cref="SeededBackend"/> answers before the call returns, so every resolve through it is trivially the
/// latest and the ordering guards never trip.
/// </summary>
public sealed class SetupResolveTests
{
    private static FieldViewModel Select(SetupViewModel flow, string key)
        => flow.Quality.Selects.Single(field => field.Key == key);

    private static string PickedValue(SetupViewModel flow, string key)
        => Select(flow, key).Options.Single(option => option.IsSelected).Value;

    /// <summary>Moves one dropdown, without waiting for the form that answers the move.</summary>
    private static void Choose(SetupViewModel flow, string key, string value)
        => Select(flow, key).Options.Single(option => option.Value == value).Choose.Execute(null);

    private static SetupViewModel Flow(DeferredBackend backend) => Flows.Setup(backend);

    /// <summary>
    /// Reads every state once and stops before the reconnect delay, so nothing is left dialling behind the
    /// assertions.
    /// Not awaited: the reads land before <c>Start</c> returns, and the task it hands back is the loop that
    /// follows the event stream.
    /// </summary>
    private static void Load(Session session)
    {
        session.Start();
        session.Stop();
    }

    /// <summary>
    /// Unresolved is a state and not a gap: every group draws its unresolved branch, because a render pass
    /// that waited for the socket would be a window that does not paint.
    /// </summary>
    [Fact]
    public void BeforeTheFirstFormLandsTheFlowDrawsNoGroupAndPublishesNothing()
    {
        var flow = Flow(new DeferredBackend());

        Assert.False(flow.Quality.IsResolved);
        Assert.False(flow.IsPublishable);
        Assert.Equal("", flow.Headline);
        Assert.Empty(flow.Steps);
        Assert.False(flow.Settled.IsCompleted);
    }

    [Fact]
    public async Task TheFirstAnswerPutsTheFormOnScreen()
    {
        var backend = new DeferredBackend();
        var flow = Flow(backend);

        await backend.AnswerAsync(0);

        Assert.True(flow.Settled.IsCompleted);
        Assert.True(flow.Quality.IsResolved);
        Assert.True(flow.IsPublishable);
        Assert.NotEmpty(flow.Headline);
    }

    /// <summary>
    /// A resolve is side-effect free and answers the same form for the same draft, so asking again buys
    /// nothing.
    /// The render function runs on every notification, which is dozens of times a second.
    /// </summary>
    [Fact]
    public async Task RenderingAnUnchangedDraftAsksTheBackendNothing()
    {
        var backend = new DeferredBackend();
        var flow = Flow(backend);
        await backend.AnswerAsync(0);

        flow.Apply();
        flow.Apply();

        Assert.Equal(1, backend.Resolves);
        Assert.True(flow.Settled.IsCompleted);
    }

    /// <summary>A write of the value already held leaves the draft where it was.</summary>
    [Fact]
    public async Task ChoosingTheValueTheDraftAlreadyHoldsAsksNothingEither()
    {
        var backend = new DeferredBackend();
        var flow = Flow(backend);
        await backend.AnswerAsync(0);

        var picked = PickedValue(flow, "publish.fps");
        Choose(flow, "publish.fps", picked);

        Assert.Equal(1, backend.Resolves);
    }

    [Fact]
    public async Task AChangedDraftAsksAgainAndTheAnswerRedrawsTheScreen()
    {
        var backend = new DeferredBackend();
        var flow = Flow(backend);
        await backend.AnswerAsync(0);

        Choose(flow, "publish.fps", "30");
        Assert.Equal(2, backend.Resolves);

        // The render pass draws what the backend has said, never what it is about to say.
        Assert.Equal("60", PickedValue(flow, "publish.fps"));

        await backend.AnswerAsync(1);
        Assert.Equal("30", PickedValue(flow, "publish.fps"));
    }

    /// <summary>A superseded resolve is asked to stop rather than left to finish unwatched.</summary>
    [Fact]
    public async Task SupersedingADraftCancelsTheResolveItReplaced()
    {
        var backend = new DeferredBackend();
        var flow = Flow(backend);
        await backend.AnswerAsync(0);

        Choose(flow, "publish.fps", "30");
        Choose(flow, "publish.fps", "24");

        Assert.True(backend.IsCancelled(1));
        Assert.False(backend.IsCancelled(2));
    }

    /// <summary>
    /// Why the token alone is not enough: cancellation is cooperative, so a superseded call can already hold
    /// its answer and deliver it after the newer one.
    /// The request number is what drops it.
    /// </summary>
    [Fact]
    public async Task AStaleAnswerArrivingLastDoesNotOverwriteTheNewerForm()
    {
        var backend = new DeferredBackend();
        var flow = Flow(backend);
        await backend.AnswerAsync(0);

        Choose(flow, "publish.fps", "30");
        Choose(flow, "publish.fps", "24");
        Assert.Equal(3, backend.Resolves);

        // Answer for the newer draft first, then for the one it superseded.
        await backend.AnswerAsync(2);
        Assert.Equal("24", PickedValue(flow, "publish.fps"));

        await backend.AnswerAsync(1);

        Assert.Equal("24", PickedValue(flow, "publish.fps"));
        Assert.Equal(3, backend.Resolves);
    }

    /// <summary>
    /// Adopting an answer is not a draft change.
    /// Treating it as one would ask again for the form already on screen, at two round trips per keystroke.
    /// </summary>
    [Fact]
    public async Task AdoptingAnAnswerDoesNotAskForItAgain()
    {
        var backend = new DeferredBackend();
        var flow = Flow(backend);

        await backend.AnswerAsync(0);
        Assert.Equal(1, backend.Resolves);

        Choose(flow, "publish.fps", "30");
        await backend.AnswerAsync(1);

        Assert.Equal(2, backend.Resolves);
        Assert.Equal(30, backend.Draft(1).Publish.Fps);
    }

    /// <summary>
    /// What the encoder probe needs: a resolve reads what has been probed rather than probing, so the forms
    /// answered before the probe lands grey no codec for missing hardware.
    /// The round-trip guard is what would keep them on screen, since the draft has not moved.
    /// </summary>
    [Fact]
    public async Task NewsThatTheAnswerMovedMakesTheFlowAskAgainForTheSameDraft()
    {
        var backend = new DeferredBackend();
        var flow = Flow(backend);
        await backend.AnswerAsync(0);

        backend.Announce();

        Assert.Equal(2, backend.Resolves);
        Assert.Equal(backend.Draft(0), backend.Draft(1));

        await backend.AnswerAsync(1);
        Assert.True(flow.Quality.IsResolved);
    }

    /// <summary>
    /// A failure that re-armed the round-trip guard would make the next render pass resolve, fail and render
    /// again, hammering an absent socket for as long as the window is open.
    /// </summary>
    [Fact]
    public void ABackendThatCannotAnswerSaysSoAndIsNotAskedAgainByRendering()
    {
        var backend = new DeferredBackend();
        var flow = Flow(backend);

        backend.Fail(0, "The backend is not running: nothing is listening on the control socket.");

        Assert.True(flow.IsUnavailable);
        Assert.Equal("The backend is not running: nothing is listening on the control socket.", flow.Unavailable);
        Assert.False(flow.Quality.IsResolved);

        flow.Apply();
        flow.Apply();

        Assert.Equal(1, backend.Resolves);
    }

    /// <summary>
    /// The settings read has no draft in front of it, so a failure on the first one leaves nothing for a
    /// draft change to restart.
    /// The notice clears because every output is written on every pass, including the branch that turns it
    /// off.
    /// </summary>
    [Fact]
    public async Task LookingAgainAsksOnceMoreAndTheAnswerClearsTheNotice()
    {
        var backend = new DeferredBackend();
        var flow = Flow(backend);
        backend.Fail(0, "The backend is not running.");

        Assert.True(flow.RetryCommand.CanExecute(null));
        flow.RetryCommand.Execute(null);
        Assert.Equal(2, backend.Resolves);

        await backend.AnswerAsync(1);

        Assert.False(flow.IsUnavailable);
        Assert.Equal("", flow.Unavailable);
        Assert.True(flow.Quality.IsResolved);
        Assert.False(flow.RetryCommand.CanExecute(null));
    }

    /// <summary>
    /// The case nearly every start meets: the app launches the backend and reaches it a moment later, so the
    /// flow's opening read is the one call that fails.
    /// The session dials every couple of seconds either way, so the recovery is news the flow listens to
    /// rather than a timer of its own.
    /// </summary>
    [Fact]
    public async Task ABackendThatCameBackIsAskedAgainWithoutTheButton()
    {
        var backend = new DeferredBackend { IsAbsent = true };
        var session = new Session(backend, action => action());
        var flow = Flows.Setup(backend, session);

        // The opening read never got as far as a resolve, so there is no draft a keystroke could restart it
        // from.
        Assert.True(flow.IsUnavailable);
        Assert.Equal(0, backend.Resolves);

        // Recovery is measured from the session having found it absent as well.
        Load(session);
        Assert.NotEqual("", session.Unavailable);

        backend.IsAbsent = false;
        Load(session);

        Assert.Equal("", session.Unavailable);
        Assert.Equal(1, backend.Resolves);

        await backend.AnswerAsync(0);

        Assert.False(flow.IsUnavailable);
        Assert.True(flow.Quality.IsResolved);
    }

    /// <summary>
    /// The session announces every state the backend sends while it is up, so a flow reacting to "reachable"
    /// rather than to the moment it became reachable would resolve on each of them.
    /// </summary>
    [Fact]
    public async Task ABackendThatIsSimplyUpDoesNotMakeTheFlowAskAgain()
    {
        var backend = new DeferredBackend();
        var session = new Session(backend, action => action());
        var flow = Flows.Setup(backend, session);

        await backend.AnswerAsync(0);
        Assert.Equal(1, backend.Resolves);

        Load(session);
        Load(session);

        Assert.Equal(1, backend.Resolves);
        Assert.False(flow.IsUnavailable);
    }
}
