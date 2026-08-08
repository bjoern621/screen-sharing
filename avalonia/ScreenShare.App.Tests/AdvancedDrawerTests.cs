using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Setup.AdvancedDrawer.ViewModel;
using ScreenShare.App.Features.Setup.Fields.ViewModel;
using ScreenShare.App.Features.Setup.QualityStep.ViewModel;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The advanced drawer, which draws the part of the quality group the step above it places
/// nowhere.
///
/// What these lock out is the drawer inventing a table. It once carried one - an element
/// name, five property rows and their defaults, seeded from a mockup - and every assertion
/// below is against a value some form carried, so a drawer that started printing figures of
/// its own would fail here rather than on someone's screen (docs/ipc-api.md, "The rule").
///
/// The group is driven directly rather than through a backend, because what is under test is
/// the split between the two layouts and not a resolve. The last test does use the flow, and
/// it is the one that states the split as an invariant.
/// </summary>
public sealed class AdvancedDrawerTests
{
    /// <summary>A group carrying the fields given, rendered once, with the writes it reports collected.</summary>
    private static (FieldGroupViewModel Group, List<(string Key, FieldValue Value)> Writes) GroupOf(params Field[] fields)
    {
        var writes = new List<(string, FieldValue)>();
        var group = new FieldGroupViewModel((key, value) => writes.Add((key, value)));

        var resolved = new FieldGroup { Key = "quality" };
        resolved.Fields.Add(fields);
        group.Apply(resolved, Vocabulary.Empty, null);

        return (group, writes);
    }

    private static Field Number(string key, long value, Unit unit = Unit.Unspecified) => new()
    {
        Key = key,
        Control = ControlKind.Number,
        Unit = unit,
        Visible = true,
        Enabled = true,
        Value = new FieldValue { Number = value },
    };

    private static Field Select(string key, string value) => new()
    {
        Key = key,
        Control = ControlKind.Select,
        Visible = true,
        Enabled = true,
        Value = new FieldValue { Text = value },
        Options = { new FieldOption { Value = value, Enabled = true } },
    };

    /// <summary>A number carrying a ladder: both halves filled, which is what the kind means.</summary>
    private static Field NumberSelect(string key, long value, params long[] ladder)
    {
        var field = new Field
        {
            Key = key,
            Control = ControlKind.NumberSelect,
            Visible = true,
            Enabled = true,
            Value = new FieldValue { Number = value },
            Range = new NumericRange { Min = 1, Max = 1000, Step = 1 },
        };
        foreach (var step in ladder)
        {
            field.Options.Add(new FieldOption { Value = step.ToString(), Enabled = true });
        }

        return field;
    }

    [Fact]
    public void TheDrawerDrawsTheNumbersTheStepPlacesNowhere()
    {
        var (group, _) = GroupOf(
            Select("codec", "libx264"),
            Number("gop", 120, Unit.Frames),
            Number("bframes", 0, Unit.Frames));

        var drawer = new AdvancedDrawerViewModel(group);

        Assert.True(drawer.HasRows);
        Assert.Equal(["gop", "bframes"], drawer.Rows.Select(row => row.Key));
        Assert.Equal("2 settings", drawer.CountLabel);
    }

    /// <summary>
    /// Every string in a row is the form's. The label, the unit and the note are read back
    /// as they arrived, which is the whole of what replaced the mockup's table.
    /// </summary>
    [Fact]
    public void ARowRepeatsWhatTheFormSaidAboutTheField()
    {
        var (group, _) = GroupOf(Number("vbv_ms", 2000, Unit.Milliseconds));

        var row = new AdvancedDrawerViewModel(group).Rows.Single();

        // The value and the unit are the form's; the heading and the paragraph are this
        // side's, looked up by the key the form named the field by.
        Assert.Equal("2000", row.Readback);
        Assert.Equal(Fields.Of("vbv_ms").Label, row.Label);
        Assert.Equal(Fields.Of("vbv_ms").Help, row.Help);
        Assert.Equal("ms", row.Unit);
    }

    /// <summary>
    /// The rows are controls rather than a printout: moving one reports the settings field
    /// it edits, which is what the old table could not do at all.
    /// </summary>
    [Fact]
    public void MovingARowReportsTheSettingsFieldItEdits()
    {
        var (group, writes) = GroupOf(Number("bframes", 0, Unit.Frames));

        new AdvancedDrawerViewModel(group).Rows.Single().Number = 2;

        var (key, value) = Assert.Single(writes);
        Assert.Equal("bframes", key);
        Assert.Equal(2L, value.Number);
    }

    /// <summary>
    /// A number carrying a ladder is one control and not two. It is drawn where typed values
    /// are drawn, and it is neither of the two kinds it is built from: a renderer that read it
    /// as a plain number would drop the ladder, and one that read it as a select would put a
    /// dropdown on the screen with no box to type a rate the ladder does not carry.
    /// </summary>
    [Fact]
    public void ANumberCarryingALadderIsOneControlAndNotTwo()
    {
        var (group, _) = GroupOf(NumberSelect("fps", 60, 30, 60, 120));

        var row = new AdvancedDrawerViewModel(group).Rows.Single();

        Assert.True(row.IsNumberSelect);
        Assert.False(row.IsNumber);
        Assert.False(row.IsSelect);
        Assert.False(row.IsChoice);
        Assert.Equal(60m, row.Number);
        Assert.Equal(["30", "60", "120"], row.Options.Select(option => option.Value));
        Assert.Single(row.Options, option => option.IsSelected);
    }

    /// <summary>
    /// The two halves write the same setting, and both write it as a number. Picking a step
    /// through a control whose options are strings is where a value silently becomes zero,
    /// which is the whole reason the field carries the kind its value arrived in.
    /// </summary>
    [Fact]
    public void BothHalvesOfALadderedNumberWriteTheSameNumber()
    {
        var (group, writes) = GroupOf(NumberSelect("fps", 60, 30, 60, 120));
        var row = new AdvancedDrawerViewModel(group).Rows.Single();

        row.Options.Single(option => option.Value == "120").Choose.Execute(null);
        row.Number = 37;

        Assert.Equal([("fps", 120L), ("fps", 37L)], writes.Select(write => (write.Key, write.Value.Number)));
    }

    /// <summary>A group whose every field the step places leaves the drawer undrawn rather than empty.</summary>
    [Fact]
    public void AGroupOfNothingButDropdownsDrawsNoDrawer()
    {
        var (group, _) = GroupOf(Select("codec", "libx264"));

        var drawer = new AdvancedDrawerViewModel(group);

        Assert.False(drawer.HasRows);
        Assert.Empty(drawer.Rows);
    }

    /// <summary>
    /// Rendering twice over an unchanged group produces the same rows, which is what lets a
    /// spinner keep its caret across a pass (docs/development-principles.md, "Idempotency").
    /// </summary>
    [Fact]
    public void ASecondRenderPassLeavesTheRowsAlone()
    {
        var (group, _) = GroupOf(Number("gop", 120, Unit.Frames));
        var drawer = new AdvancedDrawerViewModel(group);
        var before = drawer.Rows.ToList();

        drawer.Apply();

        Assert.Equal(before, drawer.Rows);
    }

    /// <summary>
    /// The invariant the two layouts share, stated against a whole flow: a control the
    /// backend offered is reachable in exactly one of them. A field in neither is a setting
    /// nobody can edit; a field in both is one setting edited from two places.
    /// </summary>
    [Fact]
    public async Task EveryFieldOfTheGroupIsDrawnExactlyOnce()
    {
        var backend = new SeededBackend("linux");
        var flow = new SetupViewModel(backend, new Session(backend, action => action()), action => action());
        await flow.Settled;

        var offered = (await backend.ResolveFormAsync(await backend.SettingsAsync()))
            .Groups.Single(group => group.Key == "quality")
            .Fields.Where(field => field.Visible)
            .Select(field => field.Key);

        var drawn = flow.Quality.Selects.Select(field => field.Key)
            .Concat(flow.Advanced.Rows.Select(row => row.Key))
            .Concat([flow.Quality.Mode!.Key, flow.Quality.Quantizer!.Key])
            .ToList();

        Assert.Equal(drawn.Count, drawn.Distinct().Count());
        Assert.Equal(offered.OrderBy(key => key), drawn.OrderBy(key => key));
    }
}
