using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.Fields.ViewModel;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// What the setup flow does with a backend that answers over a wire rather than out of a
/// dictionary: the render pass stays synchronous, the form it draws is the last one that
/// landed, and two drafts in flight at once resolve to the newer one whatever order their
/// answers arrive in.
///
/// These are the cases <see cref="SeededBackend"/> cannot produce. It answers before the call
/// returns, so every resolve through it is trivially in order and trivially the latest - which
/// is exactly why the guard it would never trip needs a backend that holds its answers
/// (docs/ipc-api.md, "The format, and why this one").
/// </summary>
public sealed class SetupResolveTests
{
    private static FieldViewModel Select(SetupViewModel flow, string key)
        => flow.Quality.Selects.Single(field => field.Key == key);

    private static string PickedValue(SetupViewModel flow, string key)
        => Select(flow, key).Options.Single(option => option.IsSelected).Value;

    /// <summary>Moves one dropdown to a value, without waiting for the form that answers it.</summary>
    private static void Choose(SetupViewModel flow, string key, string value)
        => Select(flow, key).Options.Single(option => option.Value == value).Choose.Execute(null);

    private static SetupViewModel Flow(DeferredBackend backend)
        => new(backend, new Session(backend, action => action()), action => action());

    /// <summary>
    /// The state the flow is in before anything has answered, and it is a state rather than a
    /// gap: the window is complete and every group draws its unresolved branch, because a
    /// render pass that waited for the socket would be a window that does not paint.
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
    /// Rendering asks nothing when the draft has not moved. The contract states the resolve is
    /// side-effect free and answers the same form for the same draft, so a pass that asked
    /// again would be paying for an answer it already has - and the render function is called
    /// on every notification, so "again" means dozens of times a second.
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

    /// <summary>
    /// The same for a write that changes nothing: choosing the value the draft already holds
    /// leaves the draft where it was, and a draft that has not moved is a draft the backend has
    /// already answered for.
    /// </summary>
    [Fact]
    public async Task ChoosingTheValueTheDraftAlreadyHoldsAsksNothingEither()
    {
        var backend = new DeferredBackend();
        var flow = Flow(backend);
        await backend.AnswerAsync(0);

        var picked = PickedValue(flow, "fps");
        Choose(flow, "fps", picked);

        Assert.Equal(1, backend.Resolves);
    }

    [Fact]
    public async Task AChangedDraftAsksAgainAndTheAnswerRedrawsTheScreen()
    {
        var backend = new DeferredBackend();
        var flow = Flow(backend);
        await backend.AnswerAsync(0);

        Choose(flow, "fps", "30");
        Assert.Equal(2, backend.Resolves);

        // Still the old form until the new one lands: the render pass draws what the backend
        // has said, never what it is about to say.
        Assert.Equal("60", PickedValue(flow, "fps"));

        await backend.AnswerAsync(1);
        Assert.Equal("30", PickedValue(flow, "fps"));
    }

    /// <summary>A superseded resolve is asked to stop, rather than left to finish unwatched.</summary>
    [Fact]
    public async Task SupersedingADraftCancelsTheResolveItReplaced()
    {
        var backend = new DeferredBackend();
        var flow = Flow(backend);
        await backend.AnswerAsync(0);

        Choose(flow, "fps", "30");
        Choose(flow, "fps", "24");

        Assert.True(backend.IsCancelled(1));
        Assert.False(backend.IsCancelled(2));
    }

    /// <summary>
    /// The case the request number exists for, and the reason the token alone is not enough:
    /// cancellation is cooperative, so a superseded call can already have its answer in hand
    /// and deliver it after the newer one. An out-of-order answer is dropped, and the screen
    /// keeps the form the reader's latest draft resolved to.
    /// </summary>
    [Fact]
    public async Task AStaleAnswerArrivingLastDoesNotOverwriteTheNewerForm()
    {
        var backend = new DeferredBackend();
        var flow = Flow(backend);
        await backend.AnswerAsync(0);

        Choose(flow, "fps", "30");
        Choose(flow, "fps", "24");
        Assert.Equal(3, backend.Resolves);

        // The newer draft's answer first, then the one it superseded.
        await backend.AnswerAsync(2);
        Assert.Equal("24", PickedValue(flow, "fps"));

        await backend.AnswerAsync(1);

        Assert.Equal("24", PickedValue(flow, "fps"));
        Assert.Equal(3, backend.Resolves);
    }

    /// <summary>
    /// Adopting an answer is not itself a draft change. A flow that treated the settings it
    /// just adopted as something new would ask again for the form it is already drawing, and
    /// every keystroke would cost two round trips instead of one.
    /// </summary>
    [Fact]
    public async Task AdoptingAnAnswerDoesNotAskForItAgain()
    {
        var backend = new DeferredBackend();
        var flow = Flow(backend);

        await backend.AnswerAsync(0);
        Assert.Equal(1, backend.Resolves);

        Choose(flow, "fps", "30");
        await backend.AnswerAsync(1);

        Assert.Equal(2, backend.Resolves);
        Assert.Equal(30, backend.Draft(1).Fps);
    }

    /// <summary>
    /// News that the backend's answer has moved makes the flow read again, with the draft
    /// exactly where it was.
    ///
    /// This is what the encoder probe needs. A resolve reads what has been probed rather than
    /// probing, so the forms of the first seconds grey no codec for missing hardware and the
    /// ones after the probe grey the codecs this machine cannot run. Without the re-read the
    /// screen would go on offering an encoder the backend has since established is not there,
    /// and the round-trip guard is exactly what would keep it there: the draft has not moved,
    /// so nothing else would ask again.
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
    /// A backend that cannot answer is a sentence on screen rather than a gap, and rendering
    /// after one does not ask again.
    ///
    /// The second half is the load-bearing one. A failure that re-armed the round-trip guard
    /// would make the next render pass start a resolve, fail, and render - a loop hammering an
    /// absent socket for as long as the window is open.
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
    /// Looking again is the reader's, and it is the whole recovery path: the settings read has
    /// no draft in front of it, so a failure on the first one leaves nothing for a draft change
    /// to restart. An answer clears the notice, which is the render function's usual property
    /// stated for this one - every output is written on every pass, including the branch that
    /// turns it off.
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
}
