using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The commit: when the one red button is pressable, what it sends, and what it says when it
/// is not.
///
/// <b>Four states have to hold at once, and none of them is this module's opinion.</b> The
/// settings publish (<c>Form.publishable</c>), the backend is answering, nothing is already on
/// the air (<c>PublishState.live</c>), and the relay answered (<c>RelayStatus.reachable</c>).
/// Each is a whole state some other side stated, so these tests state the reading and assert
/// what the button did with it - never how a rule was evaluated here.
/// </summary>
public sealed class GoLiveTests
{
    /// <summary>
    /// A flow whose first form has landed and whose session has read the running state once.
    /// Both fixtures answer from memory and the dispatcher runs inline, so what a test reads
    /// afterwards is what the render pass wrote.
    /// </summary>
    private static SetupViewModel Flow(PublishingBackend backend, out Session session)
    {
        var opened = new Session(backend, action => action());
        var flow = new SetupViewModel(backend, opened, action => action());

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
        Live = new PublishState.Types.Live { Settings = new StreamSettings { Name = name } },
    };

    [Fact]
    public void AResolvedFormAndAReachableRelayLetTheCommitBePressed()
    {
        var flow = Flow(new PublishingBackend(), out _);

        Assert.True(flow.IsPublishable);
        Assert.True(flow.Review.CanGoLive);
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
        var flow = new SetupViewModel(backend, session, action => action());

        Assert.False(flow.Review.CanGoLive);
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
        var flow = new SetupViewModel(backend, session, action => action());

        backend.Fail(0, "The backend is not running: nothing is listening on the control socket.");

        Assert.False(flow.Review.CanGoLive);
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

        Assert.False(flow.Review.CanGoLive);
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

        Assert.False(flow.Review.CanGoLive);
        Assert.NotEqual("", flow.Review.Blocked);
    }

    /// <summary>
    /// A stream already on the air locks the commit. The backend refuses a second start anyway,
    /// so the lock and the refusal are one answer - and the reader is pointed at the screen that
    /// owns stopping it.
    /// </summary>
    [Fact]
    public void AStreamAlreadyPublishingLocksTheCommit()
    {
        var backend = new PublishingBackend();
        var flow = Flow(backend, out var session);

        Assert.True(flow.Review.CanGoLive);

        backend.Publish = Live("lab04");
        Reload(session, flow);

        Assert.False(flow.Review.CanGoLive);
        Assert.NotEqual("", flow.Review.Blocked);
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

        Assert.False(flow.Review.CanGoLive);

        backend.Relay = new RelayStatus { Reachable = true };
        Reload(session, flow);

        Assert.True(flow.Review.CanGoLive);
        Assert.Equal("", flow.Review.Blocked);
    }

    /// <summary>
    /// Pressing it starts the publish on the draft that is on screen, and announces that the
    /// start went through. What crosses is a copy of the draft rather than the draft itself: the
    /// controls write that instance in place, and a keystroke arriving mid-flight would otherwise
    /// change the settings the stream is being built from.
    /// </summary>
    [Fact]
    public void PressingGoLiveStartsThePublishOnTheDraftOnScreenAndAnnouncesIt()
    {
        var backend = new PublishingBackend();
        var flow = Flow(backend, out _);

        var announced = 0;
        flow.WentLive += () => announced++;

        flow.Review.GoLiveCommand.Execute(null);

        var started = Assert.Single(backend.Started);
        Assert.Equal(flow.Review.StreamName, started.Name);
        Assert.Equal(1, announced);
        Assert.Equal("", flow.Review.Refusal);
        Assert.True(flow.Review.CanGoLive);
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

        Assert.False(flow.Review.GoLiveCommand.CanExecute(null));
        Assert.Empty(backend.Started);
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

        flow.Review.GoLiveCommand.Execute(null);

        Assert.Equal("cannot start publishing: ffmpeg is not on PATH", flow.Review.Refusal);
        Assert.True(flow.Review.HasRefusal);
        Assert.Equal(0, announced);
        Assert.True(flow.Review.CanGoLive);
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

        flow.Review.GoLiveCommand.Execute(null);
        Assert.True(flow.Review.HasRefusal);

        backend.Refusal = "";
        flow.Review.GoLiveCommand.Execute(null);

        Assert.Equal("", flow.Review.Refusal);
        Assert.False(flow.Review.HasRefusal);
        Assert.Single(backend.Started);
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
