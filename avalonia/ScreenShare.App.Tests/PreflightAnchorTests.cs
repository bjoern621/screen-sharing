using ScreenShare.App.Controls;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.StepStrip.ViewModel;
using ScreenShare.App.Mvvm;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A line naming a step is a way to that step, and a blocking one marks that step's chip.
/// A list that only says where a fault is fixed leaves the reader to find the step
/// and gives the strip nothing to show while they are standing somewhere else.
/// </summary>
public sealed class PreflightAnchorTests
{
    [Fact]
    public async Task PressingALineStandsOnTheStepThatFixesIt()
    {
        var flow = Flows.Setup(new SeededBackend("linux"));
        await flow.Settled;

        // Under the predicted rate, what the fixture warns on. The field sits on the network step.
        flow.CurrentStep = "network";
        flow.CurrentGroup!.Fields.Single(field => field.Key == RailLayout.UplinkKey).Number = 4;
        await flow.Settled;

        flow.CurrentStep = SetupSteps.SummaryKey;
        var warned = flow.Rail.Checks.Single(check => check.State == CheckState.Warned);

        Assert.Equal("network", warned.Anchor.StepKey);
        Assert.NotNull(warned.Anchor.Fix);

        warned.Anchor.Fix!.Execute(null);

        Assert.Equal("network", flow.CurrentStep);
    }

    /// <summary>
    /// Most controls stand behind the step's fold, closed on every launch.
    /// A press landing on the step with the control still hidden points at nothing.
    /// </summary>
    [Fact]
    public async Task PressingALineOpensTheFoldHidingTheControl()
    {
        var flow = Flows.Setup(new SeededBackend("linux"));
        await flow.Settled;

        flow.CurrentStep = "network";
        var group = flow.CurrentGroup!;
        group.Fields.Single(field => field.Key == RailLayout.UplinkKey).Number = 4;
        await flow.Settled;

        Assert.Contains(group.Folded, field => field.Key == RailLayout.UplinkKey);
        Assert.False(group.Fold.Shown);

        flow.CurrentStep = SetupSteps.SummaryKey;
        flow.Rail.Checks.Single(check => check.State == CheckState.Warned).Anchor.Fix!.Execute(null);

        Assert.Equal("network", flow.CurrentStep);
        Assert.True(group.Fold.Shown);
    }

    /// <summary>A pass over an unchanged list produces lines that compare equal, so the bound rows are left alone.</summary>
    [Fact]
    public async Task TheSameFaultProducesTheSameLine()
    {
        var flow = Flows.Setup(new SeededBackend("linux"));
        await flow.Settled;

        flow.CurrentStep = "network";
        flow.CurrentGroup!.Fields.Single(field => field.Key == RailLayout.UplinkKey).Number = 4;
        await flow.Settled;

        var first = flow.Rail.Checks.Single(check => check.State == CheckState.Warned);
        flow.Apply();

        Assert.Equal(first, flow.Rail.Checks.Single(check => check.State == CheckState.Warned));
    }

    [Fact]
    public void ABlockingLineMarksTheChipOfTheStepItNames()
    {
        var chips = Chips([Line(CheckState.Blocking, "network")]);

        Assert.True(Chip(chips, "network").IsBlocked);
        Assert.False(Chip(chips, "video").IsBlocked);
    }

    /// <summary>Where the commit stands, so anything blocking marks it, the lines naming no field included.</summary>
    [Fact]
    public void ABlockingLineMarksTheTerminalChipWhereverItIsFixed()
    {
        Assert.True(Chip(Chips([Line(CheckState.Blocking, "network")]), SetupSteps.SummaryKey).IsBlocked);
        Assert.True(Chip(Chips([Line(CheckState.Blocking, "")]), SetupSteps.SummaryKey).IsBlocked);
    }

    [Fact]
    public void AWarnedLineMarksNothing()
    {
        var chips = Chips([Line(CheckState.Warned, "network"), PreflightChecks.Clear]);

        foreach (var chip in chips)
        {
            Assert.False(chip.IsBlocked);
        }
    }

    private static readonly IReadOnlyList<SetupStepRow> Steps =
    [
        new() { Key = "video", Number = 1, Label = "Video" },
        new() { Key = "network", Number = 2, Label = "Network" },
        new() { Key = SetupSteps.SummaryKey, Number = 3, Label = SetupSteps.SummaryLabel },
    ];

    private static IReadOnlyList<StepChipViewModel> Chips(IReadOnlyList<PreflightCheckRow> checks)
        => StepChips.For(Steps, "video", checks, _ => "", _ => new DelegateCommand(() => { }));

    private static StepChipViewModel Chip(IReadOnlyList<StepChipViewModel> chips, string key)
        => chips.Single(chip => chip.Key == key);

    private static PreflightCheckRow Line(CheckState state, string stepKey) => new()
    {
        Text = "something the form said",
        State = state,
        Anchor = stepKey.Length == 0
            ? CheckAnchor.Nowhere
            : new CheckAnchor { Label = stepKey, StepKey = stepKey, Fix = new DelegateCommand(() => { }) },
    };
}
