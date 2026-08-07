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
