using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.AudioStep.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Audio step's layout: the group's repeated controls as rows, and what the reader can do to the list from them.
///
/// Stated against the properties the markup binds and against no value this module wrote.
/// Which entries exist, what each control is, which are greyed and how the absent kind is spelled
/// all arrive on the resolved form, so a shell inventing any of them fails here (<c>docs/ipc-api.md</c>, "The rule").
/// </summary>
public sealed class AudioStepTests
{
    private const string Absent = "none";

    private static readonly string[] Kinds = [Absent, "desktop", "application"];

    /// <summary>What a control wrote, so a press is asserted against the settings it addresses.</summary>
    private sealed record Write(string Key, FieldValue Value);

    /// <summary>
    /// A step over a list holding the kinds named,
    /// plus the form's own row past the end and the codec the group carries beside the list.
    /// Trailing row holds the absent kind, as the backend's default entry does
    /// (<c>backend/internal/settings/audio.go</c>, <c>DefaultAudioSource</c>).
    /// </summary>
    private static (AudioStepViewModel Step, List<Write> Writes) StepOver(params string[] entries)
    {
        var writes = new List<Write>();
        var group = new FieldGroupViewModel((key, value) => writes.Add(new Write(key, value)));
        var described = new FieldGroup { Key = AudioLayout.GroupKey };

        for (var entry = 0; entry <= entries.Length; entry++)
        {
            var kind = entry == entries.Length ? Absent : entries[entry];
            described.Fields.Add(Source(entry, kind));
            described.Fields.Add(Device(entry, kind != Absent));
            described.Fields.Add(Gain(entry));
            described.Fields.Add(Mute(entry));
        }

        described.Fields.Add(new Field
        {
            Key = "publish.audio_codec",
            Control = ControlKind.Select,
            Visible = true,
            Enabled = true,
            Value = new FieldValue { Text = "opus" },
            Options = { new FieldOption { Value = "opus", Enabled = true } },
        });

        group.Apply(described, Vocabulary.Empty, null);
        return (new AudioStepViewModel(group), writes);
    }

    private static Field Source(int entry, string kind)
    {
        var field = new Field
        {
            Key = $"publish.audio_sources[{entry}].source",
            Control = ControlKind.Select,
            Visible = true,
            Enabled = true,
            Value = new FieldValue { Text = kind },
        };

        foreach (var offered in Kinds)
        {
            field.Options.Add(new FieldOption { Value = offered, Enabled = true });
        }

        return field;
    }

    /// <summary>
    /// Device control, greyed on an entry naming no kind, as the form answers:
    /// the absent kind holds nothing to pick.
    /// </summary>
    private static Field Device(int entry, bool enabled) => new()
    {
        Key = $"publish.audio_sources[{entry}].device",
        Control = ControlKind.Select,
        Visible = true,
        Enabled = enabled,
        Value = new FieldValue { Text = "" },
        Reason = enabled ? null : new Text { Code = TextCode.AudioEntryNeedsSource },
        Options = { new FieldOption { Value = "", Enabled = true } },
    };

    private static Field Gain(int entry) => new()
    {
        Key = $"publish.audio_sources[{entry}].gain",
        Control = ControlKind.Slider,
        Visible = true,
        Enabled = true,
        Live = true,
        Unit = Unit.Percent,
        Range = new NumericRange { Min = 0, Max = 200, Step = 5 },
        Value = new FieldValue { Number = 100 },
    };

    private static Field Mute(int entry) => new()
    {
        Key = $"publish.audio_sources[{entry}].mute",
        Control = ControlKind.Toggle,
        Visible = true,
        Enabled = true,
        Live = true,
        Value = new FieldValue { Flag = false },
    };

    /// <summary>
    /// Row past the end of the list is what a reader grows the list by,
    /// so it draws as the button rather than a further row of controls nobody filled in.
    /// </summary>
    [Fact]
    public void TheRowPastTheEndIsTheButtonThatGrowsTheList()
    {
        var (step, _) = StepOver("desktop", "application");

        Assert.Equal(2, step.Rows.Count);
        Assert.True(step.HasAdd);
        Assert.Equal("publish.audio_sources[2].source", step.Add!.Key);
    }

    [Fact]
    public void AnEmptyListDrawsNoRowAndStillGrows()
    {
        var (step, _) = StepOver();

        Assert.Empty(step.Rows);
        Assert.False(step.HasRows);
        Assert.True(step.HasAdd);
        Assert.NotEmpty(step.EmptyLine);
    }

    /// <summary>Every control on a row addresses that row's entry, so a level moved on the second is not the first's.</summary>
    [Fact]
    public void ARowCarriesTheControlsOfItsOwnEntry()
    {
        var (step, _) = StepOver("desktop", "application");

        Assert.Equal("publish.audio_sources[1].source", step.Rows[1].Source!.Key);
        Assert.Equal("publish.audio_sources[1].device", step.Rows[1].Device!.Key);
        Assert.Equal("publish.audio_sources[1].gain", step.Rows[1].Gain!.Key);
        Assert.Equal("publish.audio_sources[1].mute", step.Rows[1].Mute!.Key);
    }

    /// <summary>A control is named and explained once over the column, rather than once per entry.</summary>
    [Fact]
    public void TheCopyIsWrittenPerControlAndDrawnOverTheColumn()
    {
        var (step, _) = StepOver("desktop", "application");

        Assert.Equal(Fields.Of(AudioLayout.GainKey).Label, step.GainLabel);
        Assert.NotEmpty(step.GainHelp);
        Assert.NotEmpty(step.SourceHelp);
        Assert.NotEmpty(step.DeviceHelp);
        Assert.NotEmpty(step.MuteHelp);
    }

    /// <summary>
    /// What a change costs the people watching is said once over the columns,
    /// naming the controls the form marked live rather than a pair this shell wrote down
    /// (<c>docs/field-availability.md</c>, "A live stream blocks no field").
    /// </summary>
    [Fact]
    public void TheLiveControlsAreNamedOnceOverTheColumns()
    {
        var (step, _) = StepOver("desktop", "application");

        Assert.True(step.HasLiveLine);
        Assert.Contains(Fields.Of(AudioLayout.GainKey).Label, step.LiveLine);
        Assert.Contains(Fields.Of(AudioLayout.MuteKey).Label, step.LiveLine);
    }

    /// <summary>
    /// Taking a source off is the write the dropdown beside it makes,
    /// and the value written is the form's trailing row rather than a kind this shell knows the name of.
    /// </summary>
    [Fact]
    public void TheButtonOnARowWritesTheAbsentKindTheFormNames()
    {
        var (step, writes) = StepOver("desktop", "application");

        Assert.True(step.Rows[0].CanRemove);
        step.Rows[0].RemoveCommand.Execute(null);

        var write = Assert.Single(writes);
        Assert.Equal("publish.audio_sources[0].source", write.Key);
        Assert.Equal(Absent, write.Value.Text);
    }

    /// <summary>A row already on the absent kind is one the write cannot move, so it offers no button.</summary>
    [Fact]
    public void ARowOnTheAbsentKindOffersNoButton()
    {
        var (step, _) = StepOver(Absent);

        Assert.False(step.Rows[0].CanRemove);
    }

    /// <summary>
    /// Group's other controls stay reachable, under the list and in the form's order:
    /// between the rows and them, every control the backend offered is drawn exactly once.
    /// </summary>
    [Fact]
    public void TheControlsBelongingToNoEntryAreDrawnUnderTheList()
    {
        var (step, _) = StepOver("desktop");

        var under = Assert.Single(step.UnderList);
        Assert.Equal("publish.audio_codec", under.Key);
    }

    /// <summary>
    /// One line per fact, not per control.
    /// An entry naming no kind greys the three controls inside it on one reason,
    /// and three copies of that sentence read as three faults.
    /// </summary>
    [Fact]
    public void ARowStatesEachReasonOnce()
    {
        var (step, _) = StepOver(Absent);

        var reason = Assert.Single(step.Rows[0].Reasons);
        Assert.NotEmpty(reason);
    }

    /// <summary>
    /// Render function is the whole of what a pass writes,
    /// so running it again on an unchanged group moves nothing (<c>docs/development-principles.md</c>, "One render function").
    /// </summary>
    [Fact]
    public void ASecondPassOverAnUnchangedGroupDrawsTheSameRows()
    {
        var (step, _) = StepOver("desktop", "application");
        var first = step.Rows.ToList();

        step.Apply();

        Assert.Equal(first, step.Rows.ToList());
    }
}
