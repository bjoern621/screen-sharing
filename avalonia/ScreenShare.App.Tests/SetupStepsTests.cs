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
/// The strip is the resolved form's groups, never a table held here.
/// Pins the defect a step table shipped with: keys naming groups no backend answers with drew empty columns,
/// the groups it missed were unreachable, and the fixture was written from the same table.
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

    [Fact]
    public async Task TheStripIsTheFormsGroupsAndOneMore()
    {
        var backend = new SeededBackend("linux");
        var form = await backend.ResolveFormAsync(await backend.SettingsAsync());
        var flow = await FlowAsync();

        // The form names the groups and this shell places them.
        var sent = form.Groups.Where(group => GroupPlacement.InSetup(group.Key)).ToList();

        Assert.Equal(sent.Count + 1, flow.Steps.Count);
        Assert.Equal(
            sent.Select(group => group.Key).Append(SetupSteps.ShareKey),
            flow.Steps.Select(step => step.Key));
        // Chip titles are this shell's, keyed by the form's group key.
        // What fits on a chip is this strip's decision, and the contract cannot see how wide one is.
        Assert.Equal(
            sent.Select(group => Fields.Group(group.Key).Title),
            flow.Steps.Take(sent.Count).Select(step => step.Label));
    }

    /// <summary>
    /// The wizard configures what this machine sends.
    /// Pins the defect that shipped: a page of watching settings inside the sending wizard, which persisted
    /// only if the reader went live.
    /// </summary>
    [Fact]
    public async Task TheWatchingGroupIsNoStepOfTheSendingWizard()
    {
        var backend = new SeededBackend("linux");
        var form = await backend.ResolveFormAsync(await backend.SettingsAsync());
        var flow = await FlowAsync();

        // The fixture carries one, so what follows tests a filter and not a form that lacked the group.
        Assert.Contains(form.Groups, group => GroupPlacement.InViewer(group.Key));

        Assert.DoesNotContain(flow.Steps, step => GroupPlacement.InViewer(step.Key));
        Assert.DoesNotContain(
            flow.Review.Tiles,
            tile => tile.Heading == Fields.Group(GroupPlacement.WatchKey).Title);

        flow.CurrentStep = GroupPlacement.WatchKey;
        Assert.Equal(flow.Steps[0].Key, flow.Steps.Single(step => step.IsCurrent).Key);
    }

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
    /// The fallback is read through rather than written back, so a group that returns puts the reader back
    /// on it.
    /// </summary>
    [Fact]
    public async Task AStepTheFormDoesNotCarryFallsBackToTheFirst()
    {
        var flow = await FlowAsync();

        flow.CurrentStep = "a-group-no-backend-answers-with";

        Assert.Equal(flow.Steps[0].Key, flow.Steps.Single(step => step.IsCurrent).Key);
        Assert.True(flow.ShowsFields);
    }

    /// <summary>One shorthand per group feeds both, so strip and review cannot name two configurations.</summary>
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

/// <summary>Every figure on the rail is the form's own estimate, never a number composed here.</summary>
public sealed class CostRailTests
{
    private static async Task<SetupViewModel> FlowAsync(SeededBackend backend)
    {
        var flow = Flows.Setup(backend);
        await flow.Settled;
        return flow;
    }

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
    /// One control per setting: the rail names the step that edits the uplink and carries no second spinner.
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
    /// Placement is this shell's and the form describes none, so the measurement rides on the field it
    /// writes.
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

    /// <summary>Uplink control, reached by moving the flow to the step the fixture's form puts it on.</summary>
    private static FieldViewModel Uplink(SetupViewModel flow)
    {
        flow.CurrentStep = "network";
        return flow.CurrentGroup!.Fields.Single(field => field.Key == RailLayout.UplinkKey);
    }

    [Fact]
    public async Task ADiagnosticBecomesALineNamingTheStepThatOwnsIt()
    {
        var flow = await FlowAsync(new SeededBackend("linux"));

        // Under the predicted rate, what the fixture warns on.
        Uplink(flow).Number = 4;
        await flow.Settled;

        var warned = flow.Rail.Checks.Single(check => check.State == CheckState.Warned);
        var owner = flow.Steps.Single(step => step.Key == "network");

        Assert.Contains("upload", warned.Text, StringComparison.OrdinalIgnoreCase);
        Assert.Contains(owner.Label, warned.FixedInStep);
        Assert.True(flow.Rail.IsOverUplink);
        Assert.Equal(flow.Rail.ChecksSummary, flow.Steps.Single(step => step.IsTerminal).Value);
    }

    [Fact]
    public async Task AFormWithNoDiagnosticsStillDrawsALine()
    {
        var flow = await FlowAsync(new SeededBackend("linux"));

        Assert.Equal(PreflightChecks.Clear, Assert.Single(flow.Rail.Checks));
        Assert.Equal("nothing to fix", flow.Rail.ChecksSummary);
    }

    /// <summary>
    /// One column, drawn the same on every step.
    /// Pins the arrangement this replaced: the checks and the presets were the terminal step's own column, so
    /// the rail disappeared on the one step that read the settings back, and a preset could be reached only by
    /// walking to the end of the flow.
    /// </summary>
    [Fact]
    public async Task TheRailIsTheSameColumnOnEveryStep()
    {
        var flow = await FlowAsync(new SeededBackend("linux"));
        await flow.Rail.Presets.Settled;

        var checks = flow.Rail.Checks.ToList();
        var presets = flow.Rail.Presets.Builtin.Select(row => row.Key).ToList();

        Assert.NotEmpty(checks);
        Assert.NotEmpty(presets);

        foreach (var step in flow.Steps.ToList())
        {
            flow.CurrentStep = step.Key;

            Assert.Equal(checks, flow.Rail.Checks);
            Assert.Equal(presets, flow.Rail.Presets.Builtin.Select(row => row.Key));
            Assert.True(flow.Rail.IsResolved);
        }
    }

    /// <summary>
    /// The refusal stands with the checks rather than under the form being edited: one place says what is
    /// owed, and it is the same place on every step.
    /// </summary>
    [Fact]
    public async Task TheRailCarriesWhyNoPipelineBuilds()
    {
        var backend = new SeededBackend("linux");
        var form = await backend.ResolveFormAsync(await backend.SettingsAsync());
        var flow = await FlowAsync(backend);

        Assert.NotEqual("", form.Summary.CommandError);
        Assert.True(flow.Rail.HasRefusal);
        Assert.Equal(form.Summary.CommandError, flow.Rail.Refusal);
    }

    /// <summary>A measured figure takes the path a typed one takes: one value, one re-resolve.</summary>
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
    /// A live stream blocks the measurement and no field (<c>docs/field-availability.md</c>), so the figure
    /// stays editable.
    /// The lock is read through on every pass, so a stream that ended puts the button back.
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

    /// <summary>Reads the running state once, stops before the reconnect delay, then renders.</summary>
    private static void Read(Session session, SetupViewModel flow)
    {
        session.Start();
        session.Stop();
        flow.Apply();
    }
}
