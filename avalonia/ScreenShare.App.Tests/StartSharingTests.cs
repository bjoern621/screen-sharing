using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Commit gate, read off five whole states this module does not decide.
/// <c>Form.publishable</c>, a backend that answers and <c>RelayStatus.reachable</c> decide whether it is pressable.
/// <c>PublishState.live</c> decides what pressing it does: start a stream, or restart the running one.
/// <c>Form.in_force</c> takes the restart away where the running one was built from the draft.
/// </summary>
public sealed class StartSharingTests
{
    /// <summary>
    /// Flow with its first form landed and one reading of the running state behind it.
    /// Fixtures answer from memory and the dispatcher runs inline, so a call returns with the render pass done.
    /// </summary>
    private static SetupViewModel Flow(PublishingBackend backend, out Session session)
        => Flow(backend, out session, out _);

    /// <summary>The same flow, with the draft in hand for the tests that move a value.</summary>
    private static SetupViewModel Flow(PublishingBackend backend, out Session session, out FormSession form)
    {
        var opened = new Session(backend, action => action());
        var draft = new FormSession(backend, opened, action => action());
        var flow = new SetupViewModel(backend, draft, opened, action => action());

        Load(opened);
        flow.Apply();

        session = opened;
        form = draft;
        return flow;
    }

    /// <summary>
    /// Reads every running state once and stops before the reconnect delay, so no poll runs behind the assertions.
    /// </summary>
    private static void Load(Session session)
    {
        session.Start();
        session.Stop();
    }

    /// <summary>Re-reads the running state and renders, as an event would.</summary>
    private static void Reload(Session session, SetupViewModel flow)
    {
        Load(session);
        flow.Apply();
    }

    private static PublishState Live(string name) => new()
    {
        Live = new PublishState.Types.Live { Publish = new PublishSettings(), StreamName = name },
    };

    /// <summary>Stream built from the settings the seed holds, which is the draft a fresh flow opens on.</summary>
    private static PublishState Running(PublishingBackend backend)
    {
        var settings = backend.SettingsAsync().Result;

        return new PublishState
        {
            Live = new PublishState.Types.Live
            {
                Publish = settings.Publish,
                Relay = settings.Relay,
                StreamName = settings.StreamName,
            },
        };
    }

    [Fact]
    public void AResolvedFormAndAReachableRelayLetTheCommitBePressed()
    {
        var flow = Flow(new PublishingBackend(), out _);

        Assert.True(flow.IsPublishable);
        Assert.True(flow.Review.CanStartSharing);
        Assert.False(flow.Review.IsBlocked);
        Assert.Equal("", flow.Review.Blocked);
    }

    /// <summary>Nothing says these settings publish until a form has landed.</summary>
    [Fact]
    public void BeforeTheFirstFormLandsTheCommitIsLocked()
    {
        var backend = new DeferredBackend();
        var session = new Session(backend, action => action());
        var flow = Flows.Setup(backend, session);

        Assert.False(flow.Review.CanStartSharing);
    }

    /// <summary>First of the three, because nothing else can be established while the backend is silent.</summary>
    [Fact]
    public void ABackendThatCannotBeReachedLocksTheCommitAndSaysSoInItsOwnWords()
    {
        var backend = new DeferredBackend();
        var session = new Session(backend, action => action());
        var flow = Flows.Setup(backend, session);

        backend.Fail(0, "The backend is not running: nothing is listening on the control socket.");

        Assert.False(flow.Review.CanStartSharing);
        Assert.Equal(
            "The backend is not running: nothing is listening on the control socket.",
            flow.Review.Blocked);
    }

    /// <summary>Reason is the relay's own: a sentence composed here describes a failure this side did not see.</summary>
    [Fact]
    public void AnUnreachableRelayLocksTheCommitAndCarriesTheRelaysReason()
    {
        var backend = new PublishingBackend
        {
            Relay = new RelayStatus { Reachable = false, Error = "dial tcp 127.0.0.1:9997: connection refused" },
        };

        var flow = Flow(backend, out _);

        Assert.False(flow.Review.CanStartSharing);
        Assert.Equal("dial tcp 127.0.0.1:9997: connection refused", flow.Review.Blocked);
    }

    /// <summary>
    /// Backend's opening snapshot is exactly this, unreachable with no reason,
    /// so the case is the first seconds of every session rather than an edge.
    /// </summary>
    [Fact]
    public void ARelaySnapshotWithNoReasonStillLocksTheCommit()
    {
        var backend = new PublishingBackend { Relay = new RelayStatus() };

        var flow = Flow(backend, out _);

        Assert.False(flow.Review.CanStartSharing);
        Assert.NotEqual("", flow.Review.Blocked);
    }

    /// <summary>
    /// <c>ApplyToStream</c> is the effect for a stream in force, so a lock here stands in front of a call that succeeds.
    /// </summary>
    [Fact]
    public void AStreamAlreadyPublishingLeavesTheCommitPressable()
    {
        var backend = new PublishingBackend();
        var flow = Flow(backend, out var session);

        Assert.True(flow.Review.CanStartSharing);

        backend.Publish = Live("lab04");
        Reload(session, flow);

        Assert.True(flow.Review.CanStartSharing);
        Assert.False(flow.Review.IsBlocked);
        Assert.Equal("", flow.Review.Blocked);
    }

    /// <summary>
    /// Asserted against <c>CommitCopy</c>'s row rather than the wording,
    /// a copy of the sentence here being free to drift from it.
    /// </summary>
    [Fact]
    public void TheCommitsWordsFollowWhatPressingItWillDo()
    {
        var backend = new PublishingBackend();
        var flow = Flow(backend, out var session);

        var start = CommitCopy.Of(PublishCommit.Start);
        Assert.Equal(start.Label, flow.Review.CommitLabel);
        Assert.Equal(start.Lead, flow.Review.PromiseLead);
        Assert.Equal(start.Tail, flow.Review.PromiseTail);

        backend.Publish = Live("lab04");
        Reload(session, flow);

        var apply = CommitCopy.Of(PublishCommit.Apply);
        Assert.Equal(apply.Label, flow.Review.CommitLabel);
        Assert.Equal(apply.Lead, flow.Review.PromiseLead);
        Assert.Equal(apply.Tail, flow.Review.PromiseTail);
        Assert.NotEqual(start.Label, apply.Label);
    }

    /// <summary>Applying restarts the stream, so copy reading as a uninterrupted swap misleads.</summary>
    [Fact]
    public void TheApplyWordingSaysTheStreamRestarts()
    {
        var apply = CommitCopy.Of(PublishCommit.Apply);

        Assert.Contains("restart", apply.Label, StringComparison.OrdinalIgnoreCase);
        Assert.Contains("restarts the stream", apply.Lead, StringComparison.OrdinalIgnoreCase);
    }

    /// <summary>
    /// Which effect it is comes off the running state on the press's own pass,
    /// so the label read and the call sent cannot come apart.
    /// </summary>
    [Fact]
    public void PressingTheCommitWhileAStreamIsLiveAppliesToItRatherThanStartingAnother()
    {
        var backend = new PublishingBackend();
        var flow = Flow(backend, out var session);

        backend.Publish = Live("lab04");
        Reload(session, flow);

        flow.Review.StartSharingCommand.Execute(null);

        var applied = Assert.Single(backend.Applied);
        Assert.Equal(flow.Review.StreamName, applied.StreamName);
        Assert.Empty(backend.Started);
        Assert.Equal("", flow.Review.Refusal);
    }

    /// <summary>Every output is written on every pass, the branch that turns one back included.</summary>
    [Fact]
    public void AStreamThatEndedPutsTheCommitBackToAStart()
    {
        var backend = new PublishingBackend { Publish = Live("lab04") };
        var flow = Flow(backend, out var session);

        Assert.Equal(CommitCopy.Of(PublishCommit.Apply).Label, flow.Review.CommitLabel);

        backend.Publish = new PublishState();
        Reload(session, flow);

        Assert.Equal(CommitCopy.Of(PublishCommit.Start).Label, flow.Review.CommitLabel);

        flow.Review.StartSharingCommand.Execute(null);

        Assert.Single(backend.Started);
        Assert.Empty(backend.Applied);
    }

    /// <summary>
    /// Applying restarts the stream, so a draft the stream was built from has nothing to apply:
    /// the button greys and the card says why in place of a promise about a press it does not offer.
    /// </summary>
    [Fact]
    public void ADraftTheStreamWasBuiltFromGreysTheCommit()
    {
        var backend = new PublishingBackend();
        backend.Publish = Running(backend);
        var flow = Flow(backend, out _);

        Assert.False(flow.Review.CanStartSharing);
        Assert.False(flow.Review.StartSharingCommand.CanExecute(null));
        Assert.True(flow.Review.IsInForce);
        Assert.Equal(CommitCopy.Of(PublishCommit.Apply).InForce, flow.Review.InForce);
        Assert.False(flow.Review.ShowsPromise);
        Assert.False(flow.Review.IsBlocked);
        Assert.Equal(CommitCopy.Of(PublishCommit.Apply).Label, flow.Review.CommitLabel);
    }

    /// <summary>
    /// Read off the draft on every pass, so a value moved and put back leaves nothing to apply again,
    /// with nothing here having remembered an edit (<c>docs/development-principles.md</c>, "Stateless").
    /// </summary>
    [Fact]
    public void AValueMovedLightsTheCommitAndPutBackGreysItAgain()
    {
        var backend = new PublishingBackend();
        backend.Publish = Running(backend);
        var flow = Flow(backend, out _, out var form);
        var fps = form.Draft!.Publish.Fps;

        Assert.False(flow.Review.CanStartSharing);

        // A frame rate the seed offers, its select refusing a value off its ladder.
        form.Write("publish.fps", new FieldValue { Number = 120 });

        Assert.True(flow.Review.CanStartSharing);
        Assert.False(flow.Review.IsInForce);
        Assert.True(flow.Review.ShowsPromise);

        form.Write("publish.fps", new FieldValue { Number = fps });

        Assert.False(flow.Review.CanStartSharing);
        Assert.True(flow.Review.IsInForce);
        Assert.False(flow.Review.ShowsPromise);
    }

    /// <summary>
    /// The verdict rides the form and the form is asked about the draft,
    /// so a stream that started under an unchanged draft is asked about too:
    /// what publishes is the resolve's second input, and it moved.
    /// </summary>
    [Fact]
    public void AStreamThatStartedOnTheDraftGreysTheCommitWithNothingTyped()
    {
        var backend = new PublishingBackend();
        var flow = Flow(backend, out var session);

        Assert.True(flow.Review.CanStartSharing);

        backend.Publish = Running(backend);
        Reload(session, flow);

        Assert.False(flow.Review.CanStartSharing);
        Assert.True(flow.Review.IsInForce);

        backend.Publish = new PublishState();
        Reload(session, flow);

        Assert.True(flow.Review.CanStartSharing);
        Assert.False(flow.Review.IsInForce);
        Assert.True(flow.Review.ShowsPromise);
    }

    /// <summary>
    /// A form's verdict outlives the stream it was read against: the two readings cross where a stream ends
    /// between the resolve and this pass, and a start is offered on the draft either way.
    /// </summary>
    [Fact]
    public void AVerdictWithNoStreamBehindItGreysNothing()
    {
        var gate = PublishGate.Of(
            true, inForce: true, "", publish: null, new RelayStatus { Reachable = true }, starting: false);

        Assert.Equal(PublishCommit.Start, gate.Commit);
        Assert.False(gate.InForce);
        Assert.True(gate.CanStartSharing);
    }

    /// <summary>
    /// Gate is a record so two readings compare equal,
    /// letting the review render twice and write nothing (<c>docs/development-principles.md</c>, "Idempotency").
    /// </summary>
    [Fact]
    public void TwoReadingsOfOneStateProduceTheSameGate()
    {
        var publish = Live("lab04");
        var relay = new RelayStatus { Reachable = true };

        Assert.Equal(
            PublishGate.Of(true, false, "", publish, relay, starting: false),
            PublishGate.Of(true, false, "", publish, relay, starting: false));
    }

    /// <summary>The pass runs on every keystroke, so an unchanged commit has to notify nothing.</summary>
    [Fact]
    public void ASecondRenderPassOverAnUnchangedCommitNotifiesNothing()
    {
        var backend = new PublishingBackend { Publish = Live("lab04") };
        var flow = Flow(backend, out _);

        var moved = new List<string?>();
        flow.Review.PropertyChanged += (_, e) => moved.Add(e.PropertyName);

        flow.Apply();
        flow.Apply();

        Assert.Empty(moved);
    }

    [Fact]
    public void ARelayThatCameBackUnlocksTheCommit()
    {
        var backend = new PublishingBackend { Relay = new RelayStatus { Reachable = false, Error = "no relay" } };
        var flow = Flow(backend, out var session);

        Assert.False(flow.Review.CanStartSharing);

        backend.Relay = new RelayStatus { Reachable = true };
        Reload(session, flow);

        Assert.True(flow.Review.CanStartSharing);
        Assert.Equal("", flow.Review.Blocked);
    }

    /// <summary>
    /// A copy of the draft crosses rather than the draft itself, which the controls write in place,
    /// so a keystroke mid-flight cannot move the settings the stream is built from.
    /// </summary>
    [Fact]
    public void PressingStartSharingStartsThePublishOnTheDraftOnScreenAndAnnouncesIt()
    {
        var backend = new PublishingBackend();
        var flow = Flow(backend, out _);

        var announced = 0;
        flow.WentLive += () => announced++;

        flow.Review.StartSharingCommand.Execute(null);

        var started = Assert.Single(backend.Started);
        Assert.Equal(flow.Review.StreamName, started.StreamName);
        Assert.Empty(backend.Applied);
        Assert.Equal(1, announced);
        Assert.Equal("", flow.Review.Refusal);
        Assert.True(flow.Review.CanStartSharing);
    }

    /// <summary>
    /// Command's guard is the fact the view binds, so a press getting through is one the screen never offered.
    /// </summary>
    [Fact]
    public void ALockedCommitStartsNothing()
    {
        var backend = new PublishingBackend { Relay = new RelayStatus { Reachable = false, Error = "no relay" } };
        var flow = Flow(backend, out _);

        Assert.False(flow.Review.StartSharingCommand.CanExecute(null));
        Assert.Empty(backend.Started);
        Assert.Empty(backend.Applied);
    }

    /// <summary>
    /// A start that failed leaves no state here,
    /// so the button stays pressable for a reader who fixed what the sentence named.
    /// </summary>
    [Fact]
    public void ARefusedStartShowsTheBackendsSentenceAndLeavesTheButtonPressable()
    {
        var backend = new PublishingBackend { Refusal = "cannot start publishing: ffmpeg is not on PATH" };
        var flow = Flow(backend, out _);

        var announced = 0;
        flow.WentLive += () => announced++;

        flow.Review.StartSharingCommand.Execute(null);

        Assert.Equal("cannot start publishing: ffmpeg is not on PATH", flow.Review.Refusal);
        Assert.True(flow.Review.HasRefusal);
        Assert.Equal(0, announced);
        Assert.True(flow.Review.CanStartSharing);
        Assert.Empty(backend.Started);
    }

    [Fact]
    public void AStartThatWentThroughClearsTheRefusalBeforeIt()
    {
        var backend = new PublishingBackend { Refusal = "cannot start publishing: ffmpeg is not on PATH" };
        var flow = Flow(backend, out _);

        flow.Review.StartSharingCommand.Execute(null);
        Assert.True(flow.Review.HasRefusal);

        backend.Refusal = "";
        flow.Review.StartSharingCommand.Execute(null);

        Assert.Equal("", flow.Review.Refusal);
        Assert.False(flow.Review.HasRefusal);
        Assert.Single(backend.Started);
    }

    /// <summary>
    /// One field carries both: the wait the control draws, and the guard the second press is refused off.
    /// A busy button is a call in flight, and a stream cannot be asked for twice.
    /// </summary>
    [Fact]
    public void AStartThatHasNotBeenAnsweredSaysSoAndTakesNoSecondPress()
    {
        var backend = new PublishingBackend();
        backend.HoldStarts();

        var flow = Flow(backend, out _);

        flow.Review.StartSharingCommand.Execute(null);

        Assert.True(flow.Review.StartSharingCommand.IsRunning);
        Assert.False(flow.Review.StartSharingCommand.CanExecute(null));
        Assert.False(flow.Review.CanStartSharing);

        flow.Review.StartSharingCommand.Execute(null);
        Assert.Single(backend.Started);

        backend.AnswerStarts();

        Assert.False(flow.Review.StartSharingCommand.IsRunning);
        Assert.True(flow.Review.CanStartSharing);
    }

    /// <summary>The promised name is the path a viewer asks for, computed rather than typed.</summary>
    [Fact]
    public void TheCommitNamesTheStreamTheDraftCarries()
    {
        var backend = new PublishingBackend();
        var flow = Flow(backend, out _);

        Assert.True(flow.Review.HasStreamName);
        Assert.NotEqual("", flow.Review.StreamName);
    }
}
