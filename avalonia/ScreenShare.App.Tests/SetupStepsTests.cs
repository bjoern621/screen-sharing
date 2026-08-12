using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Copy;
using ScreenShare.App.Controls;
using ScreenShare.App.Features.Fields.Model;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Where the wizard's steps come from, which is the form and not this module.
///
/// The case these lock out is the one that shipped: the flow held a table of seven steps, each
/// naming the group key it drew, and three of those keys named groups the backend does not
/// answer with - so three steps drew an empty column, and the groups the table did not name
/// were unreachable. Nothing on screen said so, and every test passed, because the fixture had
/// been written against the same table. Asserting the strip against whatever the form happens
/// to carry is what makes that impossible rather than unlikely.
/// </summary>
public sealed class SetupStepsTests
{
    private static async Task<SetupViewModel> FlowAsync()
    {
        var backend = new SeededBackend("linux");
        var flow = Flows.Setup(backend);
        await flow.Settled;
        return flow;
    }

    /// <summary>
    /// A chip per group the form carried, in the form's own order, and one more to commit on.
    /// Nothing here knows how many that is.
    /// </summary>
    [Fact]
    public async Task TheStripIsTheFormsGroupsAndOneMore()
    {
        var backend = new SeededBackend("linux");
        var form = await backend.ResolveFormAsync(await backend.SettingsAsync());
        var flow = await FlowAsync();

        // Every group except the ones another destination draws. The list is the form's and the
        // placement is this shell's, which is the whole of what this side decides.
        var sent = form.Groups.Where(group => GroupPlacement.InSetup(group.Key)).ToList();

        Assert.Equal(sent.Count + 1, flow.Steps.Count);
        Assert.Equal(
            sent.Select(group => group.Key).Append(SetupSteps.ShareKey),
            flow.Steps.Select(step => step.Key));
        // The chip's name is this side's, looked up by the key the form named the group
        // by: what fits on a chip is a decision about this strip, and the contract cannot
        // see how wide it is.
        Assert.Equal(
            sent.Select(group => Fields.Group(group.Key).Title),
            flow.Steps.Take(sent.Count).Select(step => step.Label));
    }

    /// <summary>
    /// The wizard configures what this machine sends, so the group about how it receives is not
    /// one of its steps - not as a chip, not as a review tile, and not as a group it renders.
    ///
    /// The case this locks out is the one that shipped: a page of watching settings inside the
    /// sending wizard, which a reader had to walk past to reach the commit and which only
    /// persisted if they went live.
    /// </summary>
    [Fact]
    public async Task TheWatchingGroupIsNoStepOfTheSendingWizard()
    {
        var backend = new SeededBackend("linux");
        var form = await backend.ResolveFormAsync(await backend.SettingsAsync());
        var flow = await FlowAsync();

        // The fixture carries one, so the assertions below are about a filter that ran rather
        // than about a form that never had the group.
        Assert.Contains(form.Groups, group => GroupPlacement.InViewer(group.Key));

        Assert.DoesNotContain(flow.Steps, step => GroupPlacement.InViewer(step.Key));
        Assert.DoesNotContain(
            flow.Review.Tiles,
            tile => tile.Heading == Fields.Group(GroupPlacement.WatchKey).Title);

        flow.CurrentStep = GroupPlacement.WatchKey;
        Assert.Equal(flow.Steps[0].Key, flow.Steps.Single(step => step.IsCurrent).Key);
    }

    /// <summary>Every step draws something: the group it names, or the review on the terminal one.</summary>
    [Fact]
    public async Task EveryStepInTheStripDrawsAForm()
    {
        var flow = await FlowAsync();

        foreach (var step in flow.Steps.ToList())
        {
            flow.CurrentStep = step.Key;

            if (step.Key == SetupSteps.ShareKey)
            {
                Assert.True(flow.ShowsReview);
                continue;
            }

            if (step.Key == QualityLayout.GroupKey)
            {
                Assert.True(flow.ShowsQuality);
                Assert.True(flow.Quality.IsResolved);
                continue;
            }

            Assert.True(flow.ShowsFields);
            Assert.NotNull(flow.CurrentGroup);
            Assert.True(flow.CurrentGroup!.IsResolved);
            Assert.NotEmpty(flow.CurrentGroup.Fields);
        }
    }

    /// <summary>
    /// The flow opens on the first step the form describes rather than on a key held here, and
    /// walking forward reaches the terminal step.
    /// </summary>
    [Fact]
    public async Task TheFlowOpensOnTheFirstGroupAndWalksToTheEnd()
    {
        var flow = await FlowAsync();

        Assert.Equal(flow.Steps[0].Key, flow.Steps.Single(step => step.IsCurrent).Key);
        Assert.False(flow.CanGoBack);

        while (flow.CanContinue)
        {
            flow.ContinueCommand.Execute(null);
        }

        Assert.Equal(SetupSteps.ShareKey, flow.Steps.Single(step => step.IsCurrent).Key);
        Assert.True(flow.ShowsReview);
    }

    /// <summary>
    /// A step the reader picked and a newer form no longer carries is not a dead screen: the
    /// render pass falls back to the first step. It is read through rather than written back,
    /// so a group that returns puts the reader where they were.
    /// </summary>
    [Fact]
    public async Task AStepTheFormDoesNotCarryFallsBackToTheFirst()
    {
        var flow = await FlowAsync();

        flow.CurrentStep = "a-group-no-backend-answers-with";

        Assert.Equal(flow.Steps[0].Key, flow.Steps.Single(step => step.IsCurrent).Key);
        Assert.True(flow.ShowsFields);
    }

    /// <summary>
    /// The review reads back the groups' own shorthands, which is the same sentence the strip
    /// repeats - so the two cannot name different configurations.
    /// </summary>
    [Fact]
    public async Task TheReviewReadsBackWhatTheStripSays()
    {
        var flow = await FlowAsync();

        Assert.Equal(flow.Steps.Count - 1, flow.Review.Tiles.Count);
        Assert.Equal(
            flow.Steps.Take(flow.Review.Tiles.Count).Select(step => step.Value),
            flow.Review.Tiles.Select(tile => tile.Lines));
    }
}

/// <summary>
/// The rail: what the form predicts the settings cost, and everything it said about them.
/// </summary>
public sealed class CostRailTests
{
    private static async Task<SetupViewModel> FlowAsync(SeededBackend backend)
    {
        var flow = Flows.Setup(backend);
        await flow.Settled;
        return flow;
    }

    /// <summary>
    /// Every figure on the panel is the form's estimate. It used to be the mockup's own
    /// numbers, which meant the panel read the same rate whatever the encoder was set to.
    /// </summary>
    [Fact]
    public async Task TheHeadlineFigureIsTheFormsPrediction()
    {
        var backend = new SeededBackend("linux");
        var form = await backend.ResolveFormAsync(await backend.SettingsAsync());
        var flow = await FlowAsync(backend);

        var estimate = form.Summary.Estimate;
        var uplink = estimate.BitrateMbps + estimate.HeadroomMbps;

        Assert.True(flow.Rail.IsResolved);
        Assert.Contains(estimate.BitrateMbps.ToString("0.#"), flow.Rail.Bitrate);
        Assert.Contains(uplink.ToString("0"), flow.Rail.UplinkCaption);
        Assert.False(flow.Rail.IsOverUplink);
        Assert.InRange(flow.Rail.FillShare, 0, flow.Rail.UplinkShare);
    }

    /// <summary>
    /// The uplink is read on the panel it is the limit of and edited on the step that owns the
    /// control, and the panel says which step that is. One control per setting: the rail used to
    /// carry a second spinner over the same field.
    /// </summary>
    [Fact]
    public async Task TheRailReadsTheUplinkAndNamesTheStepThatEditsIt()
    {
        var flow = await FlowAsync(new SeededBackend("linux"));
        var owner = flow.Steps.Single(step => step.Key == "network");

        Assert.True(flow.Rail.HasUplink);
        Assert.Equal(Uplink(flow).Label, flow.Rail.UplinkLabel);
        Assert.Contains(Uplink(flow).Readback, flow.Rail.UplinkFigure);
        Assert.Contains(owner.Label, flow.Rail.UplinkHint);
    }

    /// <summary>
    /// The measurement is offered beside the figure it writes, on the step that owns the field.
    /// It is the screen's own placement and not something the form described, which is why it
    /// rides on the field rather than on the panel.
    /// </summary>
    [Fact]
    public async Task TheMeasurementIsOfferedBesideTheControlItWrites()
    {
        var flow = await FlowAsync(new SeededBackend("linux"));

        Assert.True(Uplink(flow).HasAction);
        Assert.Equal("Measure", Uplink(flow).Action!.Label);

        foreach (var field in flow.CurrentGroup!.Fields.Where(field => field.Key != RailLayout.UplinkKey))
        {
            Assert.False(field.HasAction);
        }
    }

    /// <summary>The uplink control, on the step the fixture's form puts it on.</summary>
    private static FieldViewModel Uplink(SetupViewModel flow)
    {
        flow.CurrentStep = "network";
        return flow.CurrentGroup!.Fields.Single(field => field.Key == RailLayout.UplinkKey);
    }

    /// <summary>
    /// A diagnostic is a line on the rail's list, ranked by its severity and anchored to the
    /// step that owns the control it is about - which is what stops it being a dead end.
    /// </summary>
    [Fact]
    public async Task ADiagnosticBecomesALineNamingTheStepThatOwnsIt()
    {
        var flow = await FlowAsync(new SeededBackend("linux"));

        // Under the predicted rate, which is what the fixture warns about.
        Uplink(flow).Number = 4;
        await flow.Settled;

        var warned = flow.Rail.Checks.Single(check => check.State == CheckState.Warned);
        var owner = flow.Steps.Single(step => step.Key == "network");

        Assert.Contains("upload", warned.Text, StringComparison.OrdinalIgnoreCase);
        Assert.Contains(owner.Label, warned.FixedInStep);
        Assert.True(flow.Rail.IsOverUplink);
        Assert.Equal(flow.Rail.ChecksSummary, flow.Steps.Single(step => step.IsTerminal).Value);
    }

    /// <summary>Nothing to say is a line saying so, not a card that empties out.</summary>
    [Fact]
    public async Task AFormWithNoDiagnosticsStillDrawsALine()
    {
        var flow = await FlowAsync(new SeededBackend("linux"));

        Assert.Equal(PreflightChecks.Clear, Assert.Single(flow.Rail.Checks));
        Assert.Equal("nothing to fix", flow.Rail.ChecksSummary);
    }

    /// <summary>
    /// Measuring writes the figure in through the same path a typed one takes, so a measured
    /// uplink and a typed one are one value and one re-resolve.
    /// </summary>
    [Fact]
    public async Task MeasuringTheUplinkWritesTheFigureIntoTheDraft()
    {
        var flow = await FlowAsync(new SeededBackend("linux"));

        Uplink(flow).Action!.Command.Execute(null);
        await flow.Settled;

        Assert.Equal((decimal)SeededBackend.MeasuredUplinkMbps, Uplink(flow).Number);
        Assert.Contains(SeededBackend.MeasuredUplinkMbps.ToString("0"), flow.Rail.UplinkCaption);
        Assert.Contains(SeededBackend.MeasuredUplinkMbps.ToString("0"), flow.Rail.UplinkFigure);
    }

    /// <summary>
    /// A stream on the air greys the button and states why beside it, rather than leaving it
    /// pressable into a refusal the backend would send back. The figure itself stays editable: a
    /// live stream blocks the measurement and no field (<c>docs/field-availability.md</c>).
    ///
    /// The lock is read through from the running state on every pass, so a stream that ended puts
    /// the button back with nothing here having remembered that it was locked.
    /// </summary>
    [Fact]
    public void AStreamOnTheAirGreysTheMeasurementAndSaysWhyBesideIt()
    {
        var backend = new PublishingBackend { Publish = Live("lab04") };
        var session = new Session(backend, static action => action());
        var flow = Flows.Setup(backend, session);

        Read(session, flow);

        Assert.False(Uplink(flow).Action!.Command.CanExecute(null));
        Assert.True(Uplink(flow).HasActionNotice);
        Assert.Contains("stream is publishing", Uplink(flow).ActionNotice);
        Assert.True(Uplink(flow).IsEnabled);

        backend.Publish = new PublishState();
        Read(session, flow);

        Assert.True(Uplink(flow).Action!.Command.CanExecute(null));
        Assert.False(Uplink(flow).HasActionNotice);
        Assert.Equal("", Uplink(flow).ActionNotice);
    }

    private static PublishState Live(string name) => new()
    {
        Live = new PublishState.Types.Live { Publish = new PublishSettings { Name = name } },
    };

    /// <summary>
    /// Reads the running state once and stops before the reconnect delay, then renders - so what
    /// a test reads afterwards is what the render pass made of the state it set.
    /// </summary>
    private static void Read(Session session, SetupViewModel flow)
    {
        session.Start();
        session.Stop();
        flow.Apply();
    }
}
