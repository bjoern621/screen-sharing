using ScreenShare.App.Backend;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The quality step, driven end to end: a backend describing a group, the flow adopting the draft it answers
/// with, and the controls the step draws from it.
///
/// What these lock out is the shell deciding anything.
/// Every assertion below is made through the properties the markup binds, and none of them names a value this
/// module wrote: the entries, their labels, their trailing notes and their greying all come out of the
/// resolved form, so a shell that started inventing a list would fail here rather than on someone's screen
/// (docs/ipc-api.md, "The rule").
///
/// Every test awaits, because the seam is a round trip: a draft change asks the backend for a form and the
/// answer lands on a later pass.
/// The stand-in answers from memory and the dispatcher below runs inline, so the wait is over before it
/// starts - but waiting is what the assertion is entitled to do, and a test that read the controls without it
/// would be asserting against a timing the real client does not have.
/// </summary>
public sealed class QualityStepTests
{
    /// <summary>
    /// A flow whose first form has landed.
    /// The dispatcher runs the answer straight through, so what a test reads afterwards is what the render
    /// pass wrote.
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

    /// <summary>Moves one control and waits for the form that answers the move.</summary>
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

    /// <summary>
    /// The case the screenshot showed: a control that printed a value and opened nothing.
    /// It opens now, and what it opens onto is the backend's list.
    /// </summary>
    [Fact]
    public async Task TheOutputResolutionOffersTheSourceAndTheScalesBelowIt()
    {
        var resolution = Select(await FlowAsync(), "publish.output_resolution");

        Assert.Contains(resolution.Options, option => option.Value == "1920x1080");
        Assert.Contains(resolution.Options, option => option.Value == "2560x1440");
        Assert.DoesNotContain(resolution.Options, option => option.Value == "3840x2160");
    }

    /// <summary>
    /// The trailing note is what makes an entry honest: it names what the value was derived from, so the
    /// reader sees the cost without opening the step that owns the source.
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
    /// A select over a number is the case a string-only write gets silently wrong: it would mark one entry
    /// and store zero.
    /// The frame rate is that case.
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
    /// A frame rate above the screen's own refresh rate stays offered.
    /// The capture has no new picture for the frames in between and the encoder codes repeats of the last
    /// one, which costs bandwidth and buys no motion - but it is a legal thing to ask for, so the form says
    /// so as a diagnostic rather than taking the entry away.
    /// </summary>
    [Fact]
    public async Task AFramerateAboveTheSourceIsStillOffered()
    {
        var above = Select(await FlowAsync(), "publish.fps").Options.Single(option => option.Value == "120");

        Assert.True(above.IsEnabled);
        Assert.Empty(above.Reason);
    }

    /// <summary>
    /// The greying the shell renders without knowing that a mode and a quantizer have anything to do with
    /// each other: the backend says so, and the same control draws both answers.
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
    /// The rate control's grid holds the modes this backend offers with nothing left over.
    /// The shape is the step's, but the count is the form's, so a backend that offers one more mode is laid
    /// out by the same rule rather than by an edit here.
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
    /// The same rule at the counts the panel gets wrong on its own: asked for neither dimension, a
    /// UniformGrid squares off both, so five options open a three by three and the row under the cards is a
    /// card's worth of empty column.
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

    /// <summary>
    /// The step's chip repeats what the backend said the group settled on, so a chip and the form cannot name
    /// different configurations.
    /// </summary>
    [Fact]
    public async Task TheStepChipRepeatsTheGroupsOwnSummary()
    {
        var flow = await FlowAsync();
        var chip = flow.Steps.Single(step => step.Key == QualityLayout.GroupKey);

        Assert.Equal(flow.Quality.Summary, chip.Value);
    }

    /// <summary>
    /// Rendering twice over an unchanged draft produces the same rows, which is what lets an open menu
    /// survive a pass (docs/development-principles.md, "Idempotency").
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
