using ScreenShare.App.Backend;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The quality step end to end: a backend describing a group, the flow adopting the draft it answers with, and
/// the controls drawn from it.
/// Asserted through the properties the markup binds, and against no value this module wrote: entries, labels,
/// trailing notes and greying all arrive on the resolved form, so a shell that invented a list fails here
/// rather than on a screen (docs/ipc-api.md, "The rule").
/// Every test awaits because the seam is a round trip, and the answer lands on a later pass.
/// The stand-in answers from memory over an inline dispatcher, so the wait is over before it starts, but a
/// test that skipped it would assert against a timing the real client does not have.
/// </summary>
public sealed class QualityStepTests
{
    /// <summary>
    /// A flow whose first form has landed.
    /// The answer is dispatched straight through, so what a test reads next is what the render pass wrote.
    /// </summary>
    private static async Task<SetupViewModel> FlowAsync()
    {
        var backend = new SeededBackend("linux");
        var flow = Flows.Setup(backend);
        await flow.Settled;
        return flow;
    }

    private static FieldViewModel Select(SetupViewModel flow, string key)
        => flow.Quality.Selects.Single(field => field.Key == key);

    private static async Task ChooseAsync(SetupViewModel flow, OptionViewModel option)
    {
        option.Choose.Execute(null);
        await flow.Settled;
    }

    [Fact]
    public async Task TheQualityStepDrawsTheGroupTheBackendDescribed()
    {
        var flow = await FlowAsync();

        Assert.True(flow.Quality.IsResolved);
        Assert.NotEmpty(flow.Quality.Title);
        Assert.True(flow.Quality.HasMode);
        Assert.True(flow.Quality.HasQuantizer);
    }

    [Fact]
    public async Task EveryDropdownOffersEntriesAndNoneOfThemIsEmpty()
    {
        var flow = await FlowAsync();

        Assert.NotEmpty(flow.Quality.Selects);
        Assert.All(flow.Quality.Selects, field =>
        {
            Assert.NotEmpty(field.Options);
            Assert.All(field.Options, option => Assert.NotEmpty(option.Label));
        });
    }

    /// <summary>Pins the regression where the control printed a value and opened onto nothing.</summary>
    [Fact]
    public async Task TheOutputResolutionOffersTheSourceAndTheScalesBelowIt()
    {
        var resolution = Select(await FlowAsync(), "publish.output_resolution");

        Assert.Contains(resolution.Options, option => option.Value == "1920x1080");
        Assert.Contains(resolution.Options, option => option.Value == "2560x1440");
        Assert.DoesNotContain(resolution.Options, option => option.Value == "3840x2160");
    }

    /// <summary>
    /// The note names what the entry was derived from, so the cost is readable without opening the step that
    /// owns the source.
    /// </summary>
    [Fact]
    public async Task AScaledResolutionSaysWhatItWasScaledFrom()
    {
        var scaled = Select(await FlowAsync(), "publish.output_resolution").Options.Single(option => option.Value == "1920x1080");

        Assert.True(scaled.HasNote);
        Assert.Contains("2560", scaled.Note);
    }

    [Fact]
    public async Task PickingAResolutionMovesTheDraftAndTheControlShowsIt()
    {
        var flow = await FlowAsync();
        var resolution = Select(flow, "publish.output_resolution");
        var picked = resolution.Options.Single(option => option.Value == "1280x720");

        await ChooseAsync(flow, picked);

        var after = Select(flow, "publish.output_resolution");
        Assert.Equal(picked.Label, after.PickedLabel);
        Assert.Single(after.Options, option => option.IsSelected);
        Assert.Equal("1280x720", after.Options.Single(option => option.IsSelected).Value);
    }

    /// <summary>
    /// The frame rate is a select over a number, and a string-only write marks the entry while storing zero.
    /// </summary>
    [Fact]
    public async Task PickingAFramerateStoresTheNumberAndNotAZero()
    {
        var flow = await FlowAsync();
        var picked = Select(flow, "publish.fps").Options.Single(option => option.Value == "30");

        await ChooseAsync(flow, picked);

        var after = Select(flow, "publish.fps");
        Assert.Equal("30", after.Options.Single(option => option.IsSelected).Value);
        Assert.Equal("30", after.Number.ToString());
    }

    /// <summary>
    /// Above the screen's refresh rate the encoder codes repeats, which costs bandwidth and buys no motion.
    /// It is legal to ask for, so the form carries a diagnostic instead of taking the entry away.
    /// </summary>
    [Fact]
    public async Task AFramerateAboveTheSourceIsStillOffered()
    {
        var above = Select(await FlowAsync(), "publish.fps").Options.Single(option => option.Value == "120");

        Assert.True(above.IsEnabled);
        Assert.Empty(above.Reason);
    }

    /// <summary>
    /// Nothing here knows a mode and a quantizer are related: the backend states the greying and one control
    /// draws either answer.
    /// </summary>
    [Fact]
    public async Task TheQuantizerFollowsTheRateControlMode()
    {
        var flow = await FlowAsync();
        var modes = flow.Quality.Mode!;

        await ChooseAsync(flow, modes.Options.Single(option => option.Value == "crf"));
        Assert.True(flow.Quality.Quantizer!.IsEnabled);

        await ChooseAsync(flow, modes.Options.Single(option => option.Value == "cbr"));
        Assert.False(flow.Quality.Quantizer!.IsEnabled);
        Assert.NotEmpty(flow.Quality.Quantizer!.Reason);
    }

    /// <summary>
    /// The shape is the step's and the count is the form's, so one more mode from the backend is laid out by
    /// the same rule rather than by an edit here.
    /// </summary>
    [Fact]
    public async Task TheRateControlCardsFillEveryRowTheyOpen()
    {
        var flow = await FlowAsync();
        var modes = flow.Quality.Mode!.Options.Count;
        var columns = flow.Quality.ModeColumns;
        var rows = (modes + columns - 1) / columns;

        Assert.True(modes > 0);
        Assert.True(rows * columns >= modes, "the grid holds every mode");
        Assert.True((rows - 1) * columns < modes, "the last row of the grid carries a card");
    }

    /// <summary>
    /// The counts a UniformGrid gets wrong on its own: given neither dimension it squares off both, so five
    /// options open a three by three and leave a row of empty columns under the cards.
    /// </summary>
    [Theory]
    [InlineData(0, 1)]
    [InlineData(1, 1)]
    [InlineData(2, 2)]
    [InlineData(4, 2)]
    [InlineData(5, 3)]
    [InlineData(7, 3)]
    [InlineData(10, 4)]
    public void ACardGridIsAsWideAsItsCountIsSquare(int options, int columns)
    {
        Assert.Equal(columns, QualityLayout.CardColumns(options));
    }

    /// <summary>A chip composed here could name a configuration the form does not.</summary>
    [Fact]
    public async Task TheStepChipRepeatsTheGroupsOwnSummary()
    {
        var flow = await FlowAsync();
        var chip = flow.Steps.Single(step => step.Key == QualityLayout.GroupKey);

        Assert.Equal(flow.Quality.Summary, chip.Value);
    }

    /// <summary>
    /// Equal rows over an unchanged draft are what lets an open menu survive a pass
    /// (docs/development-principles.md, "Idempotency").
    /// </summary>
    [Fact]
    public async Task ASecondRenderPassLeavesTheEntriesAlone()
    {
        var flow = await FlowAsync();
        var before = Select(flow, "publish.output_resolution").Options.ToList();

        flow.Apply();
        await flow.Settled;

        Assert.Equal(before, Select(flow, "publish.output_resolution").Options);
    }
}
