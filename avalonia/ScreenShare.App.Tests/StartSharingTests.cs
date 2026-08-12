using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The commit: when the one red button is pressable, what it sends, what it calls itself, and
/// what it says when it cannot be pressed.
///
/// <b>Four states decide it, and none of them is this module's opinion.</b> The settings publish
/// (<c>Form.publishable</c>), the backend is answering, and the relay answered
/// (<c>RelayStatus.reachable</c>) are the three that let it be pressed at all. The fourth,
/// whether a stream is already on the air (<c>PublishState.live</c>), decides what pressing it
/// does rather than whether it can be done: with nothing publishing the commit starts a stream,
/// and with one running it restarts that stream on these settings. Each is a whole state some
/// other side stated, so these tests state the reading and assert what the button did with it -
/// never how a rule was evaluated here.
/// </summary>
public sealed class StartSharingTests
{
    /// <summary>
    /// A flow whose first form has landed and whose session has read the running state once.
    /// Both fixtures answer from memory and the dispatcher runs inline, so what a test reads
    /// afterwards is what the render pass wrote.
    /// </summary>
    private static SetupViewModel Flow(PublishingBackend backend, out Session session)
    {
        var opened = new Session(backend, action => action());
        var flow = Flows.Setup(backend, opened);

        Load(opened);
        flow.Apply();

        session = opened;
        return flow;
    }

    /// <summary>
    /// Reads every running state once and stops before the reconnect delay, so the states a test
    /// set are on the session and nothing is left polling behind the assertions.
    /// </summary>
    private static void Load(Session session)
    {
        session.Start();
        session.Stop();
    }

    /// <summary>Re-reads the running state after a test moved it, the way an event would.</summary>
    private static void Reload(Session session, SetupViewModel flow)
    {
        Load(session);
        flow.Apply();
    }

    private static PublishState Live(string name) => new()
    {
        Live = new PublishState.Types.Live { Publish = new PublishSettings { Name = name } },
    };

    [Fact]
    public void AResolvedFormAndAReachableRelayLetTheCommitBePressed()
    {
        var flow = Flow(new PublishingBackend(), out _);

        Assert.True(flow.IsPublishable);
        Assert.True(flow.Review.CanStartSharing);
        Assert.False(flow.Review.IsBlocked);
        Assert.Equal("", flow.Review.Blocked);
    }

    /// <summary>
    /// Before the first form there is nothing that says these settings publish, so the button is
    /// locked - and it says nothing, because nothing has been established to say.
    /// </summary>
    [Fact]
    public void BeforeTheFirstFormLandsTheCommitIsLocked()
    {
        var backend = new DeferredBackend();
        var session = new Session(backend, action => action());
        var flow = Flows.Setup(backend, session);

        Assert.False(flow.Review.CanStartSharing);
    }

    /// <summary>
    /// A backend that cannot be reached locks the commit and its own sentence is the one shown.
    /// It comes first of the three, because nothing else can be established while it holds.
    /// </summary>
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

    /// <summary>
    /// An unreachable relay locks the commit, and the reason is the relay's own rather than one
    /// composed here - a shell that wrote its own would be describing a failure it did not see.
    /// </summary>
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
    /// A relay that answered nothing at all still locks the commit, and says the one thing that
    /// is true: it could not be reached. The backend's opening snapshot is exactly this - it must
    /// not claim reachable before anything has asked - so the case is the first seconds of every
    /// session rather than an edge.
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
    /// A stream already on the air no longer locks the commit. It used to, because the only
    /// effect the shell could reach was <c>StartPublish</c> and the backend refuses that while a
    /// pipeline is in force; <c>ApplyToStream</c> is the effect for exactly that state, so
    /// refusing here would be this module standing in front of a call that would succeed.
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
    /// What a live stream changes is the commit rather than the lock, and the button says so
    /// before it is pressed. The words come out of one table read off the gate's own answer, so
    /// this states that the flow picked the right row rather than restating the wording - which
    /// would be the sentence written down in a second place, free to drift from the first.
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

    /// <summary>
    /// The apply wording says the stream restarts, in the copy itself. The backend has no
    /// live-safe change and never had one, so a reader who presses this expecting a seamless
    /// swap has been misled by the button - which is the one thing this table exists to prevent.
    /// </summary>
    [Fact]
    public void TheApplyWordingSaysTheStreamRestarts()
    {
        var apply = CommitCopy.Of(PublishCommit.Apply);

        Assert.Contains("restart", apply.Label, StringComparison.OrdinalIgnoreCase);
        Assert.Contains("restarts the stream", apply.Lead, StringComparison.OrdinalIgnoreCase);
    }

    /// <summary>
    /// Pressing it while a stream is live applies to that stream rather than asking for a second
    /// one. Which of the two it is is read off the running state on the pass the press happens
    /// on, so the label the reader saw and the call that goes out cannot come apart.
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
        Assert.Equal(flow.Review.StreamName, applied.Publish.Name);
        Assert.Empty(backend.Started);
        Assert.Equal("", flow.Review.Refusal);
    }

    /// <summary>
    /// A stream that ended puts the commit back to a start, with nothing here having remembered
    /// that it was an apply. It is the render function's own property applied to the words on the
    /// button: every output is written on every pass, including the branch that turns one back.
    /// </summary>
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
    /// Two readings of one running state produce gates that compare equal, which is what lets the
    /// review render twice over unchanged state and write nothing
    /// (docs/development-principles.md, "Idempotency"). It is a record for exactly this.
    /// </summary>
    [Fact]
    public void TwoReadingsOfOneStateProduceTheSameGate()
    {
        var publish = Live("lab04");
        var relay = new RelayStatus { Reachable = true };

        Assert.Equal(
            PublishGate.Of(true, "", publish, relay, starting: false),
            PublishGate.Of(true, "", publish, relay, starting: false));
    }

    /// <summary>
    /// A second render pass over unchanged state notifies nothing on the review, which is what
    /// makes the commit safe to reconcile from a pass that runs on every keystroke. The words on
    /// the button are the ones this covers that the earlier idempotency tests did not.
    /// </summary>
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

    /// <summary>
    /// A relay that came back unlocks the button with nothing here having remembered it was
    /// locked. That is the render function's own property applied to the commit: every output is
    /// written on every pass, including the branch that turns something back on.
    /// </summary>
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
    /// Pressing it starts the publish on the draft that is on screen, and announces that the
    /// start went through. What crosses is a copy of the draft rather than the draft itself: the
    /// controls write that instance in place, and a keystroke arriving mid-flight would otherwise
    /// change the settings the stream is being built from.
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
        Assert.Equal(flow.Review.StreamName, started.Publish.Name);
        Assert.Empty(backend.Applied);
        Assert.Equal(1, announced);
        Assert.Equal("", flow.Review.Refusal);
        Assert.True(flow.Review.CanStartSharing);
    }

    /// <summary>
    /// A locked button starts nothing. The command's own guard is what says so, and it is the
    /// same fact the view binds - so a press that got through anyway would be a press the screen
    /// never offered.
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
    /// A refusal is the backend's own sentence, shown as it stands, and it leaves the button
    /// pressable: the reader may well have fixed what it named. A start that failed is not a
    /// state this flow holds - nothing here says a stream exists.
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

    /// <summary>
    /// A start that went through clears the refusal the last one left. It is the render
    /// function's usual property stated for a string, and the reason the sentence is written on
    /// every pass rather than only when there is one.
    /// </summary>
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
    /// A start that has been asked for and not answered says so on the button, and takes no
    /// second press while it is out.
    ///
    /// The two are one fact rather than two: what the control draws its wait from is the same
    /// field the command refuses the second press off, so a button that looks busy is a call
    /// that is really in flight and a stream cannot be asked for twice.
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

    /// <summary>
    /// What the commit promises is the path a viewer will really ask for: the draft's own name,
    /// which moves when the destination step's field does.
    /// </summary>
    [Fact]
    public void TheCommitNamesTheStreamTheDraftCarries()
    {
        var backend = new PublishingBackend();
        var flow = Flow(backend, out _);

        Assert.True(flow.Review.HasStreamName);
        Assert.NotEqual("", flow.Review.StreamName);
    }
}
