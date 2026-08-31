using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Setup.AdvancedGroup.ViewModel;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.QualityStep.ViewModel;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The advanced card draws the part of the quality group the step above it places nowhere.
/// Locked out is the card inventing a value: every assertion is against one some form carried
/// (<c>docs/ipc-api.md</c>, "The rule").
/// The group is driven directly rather than through a backend,
/// the subject being the split between the two layouts and not a resolve.
/// EveryFieldOfTheGroupIsDrawnExactlyOnce is the exception, and states that split as an invariant.
/// </summary>
public sealed class AdvancedGroupTests
{
    /// <summary>Fields rendered once, with the writes the group reports collected.</summary>
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

    /// <summary>Both halves filled, what ControlKind.NumberSelect means.</summary>
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
    public void TheCardDrawsTheNumbersTheStepPlacesNowhere()
    {
        var (group, _) = GroupOf(
            Select("codec", "libx264"),
            Number("gop", 120, Unit.Frames),
            Number("bframes", 0, Unit.Frames));

        var advanced = new AdvancedGroupViewModel(group);

        Assert.True(advanced.HasRows);
        Assert.Equal(["gop", "bframes"], advanced.Rows.Select(row => row.Key));
    }

    [Fact]
    public void ARowRepeatsWhatTheFormSaidAboutTheField()
    {
        var (group, _) = GroupOf(Number("vbv_ms", 2000, Unit.Milliseconds));

        var row = new AdvancedGroupViewModel(group).Rows.Single();

        // Value and unit come off the form, heading and paragraph off this side,
        // looked up by the key the form named the field by.
        Assert.Equal("2000", row.Readback);
        Assert.Equal(Fields.Of("vbv_ms").Label, row.Label);
        Assert.Equal(Fields.Of("vbv_ms").Help, row.Help);
        Assert.Equal("ms", row.Unit);
    }

    [Fact]
    public void MovingARowReportsTheSettingsFieldItEdits()
    {
        var (group, writes) = GroupOf(Number("bframes", 0, Unit.Frames));

        new AdvancedGroupViewModel(group).Rows.Single().Number = 2;

        var (key, value) = Assert.Single(writes);
        Assert.Equal("bframes", key);
        Assert.Equal(2L, value.Number);
    }

    /// <summary>
    /// Read as a plain number the ladder is dropped,
    /// read as a select there is no box for a rate the ladder does not carry.
    /// </summary>
    [Fact]
    public void ANumberCarryingALadderIsOneControlAndNotTwo()
    {
        var (group, _) = GroupOf(NumberSelect("fps", 60, 30, 60, 120));

        var row = new AdvancedGroupViewModel(group).Rows.Single();

        Assert.True(row.IsNumberSelect);
        Assert.False(row.IsNumber);
        Assert.False(row.IsSelect);
        Assert.False(row.IsChoice);
        Assert.Equal(60m, row.Number);
        Assert.Equal(["30", "60", "120"], row.Options.Select(option => option.Value));
        Assert.Single(row.Options, option => option.IsSelected);
    }

    /// <summary>
    /// Options are strings, so a step written back as one is where a value silently becomes zero.
    /// The field carries the kind its value arrived in.
    /// </summary>
    [Fact]
    public void BothHalvesOfALadderedNumberWriteTheSameNumber()
    {
        var (group, writes) = GroupOf(NumberSelect("fps", 60, 30, 60, 120));
        var row = new AdvancedGroupViewModel(group).Rows.Single();

        row.Options.Single(option => option.Value == "120").Choose.Execute(null);
        row.Number = 37;

        Assert.Equal([("fps", 120L), ("fps", 37L)], writes.Select(write => (write.Key, write.Value.Number)));
    }

    [Fact]
    public void AGroupOfNothingButDropdownsDrawsNoCard()
    {
        var (group, _) = GroupOf(Select("codec", "libx264"));

        var advanced = new AdvancedGroupViewModel(group);

        Assert.False(advanced.HasRows);
        Assert.Empty(advanced.Rows);
    }

    /// <summary>
    /// A row surviving a pass unchanged lets a spinner keep its caret
    /// (<c>docs/development-principles.md</c>, "Idempotency").
    /// </summary>
    [Fact]
    public void ASecondRenderPassLeavesTheRowsAlone()
    {
        var (group, _) = GroupOf(Number("gop", 120, Unit.Frames));
        var advanced = new AdvancedGroupViewModel(group);
        var before = advanced.Rows.ToList();

        advanced.Apply();

        Assert.Equal(before, advanced.Rows);
    }

    /// <summary>
    /// A field in neither layout is a setting nobody can edit, one in both a setting edited from two places.
    /// Stated against a whole flow, the two layouts together having to cover the group.
    /// </summary>
    [Fact]
    public async Task EveryFieldOfTheGroupIsDrawnExactlyOnce()
    {
        var backend = new SeededBackend("linux");
        var flow = Flows.Setup(backend);
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
