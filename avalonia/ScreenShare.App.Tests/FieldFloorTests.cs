using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.AudioStep.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.QualityStep.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The wizard's floor: what a guest touches stays visible, and the rest of a step folds behind
/// a closed disclosure.
/// The fold is view state the reader set, so a render pass keeps it and nothing persists it.
/// The quality step folds whole, and the audio step folds the format alone.
/// </summary>
public sealed class FieldFloorTests
{
    private static Field Text(string key) => new()
    {
        Key = key,
        Control = ControlKind.Text,
        Visible = true,
        Enabled = true,
        Value = new FieldValue { Text = "x" },
    };

    private static Field Number(string key, long value) => new()
    {
        Key = key,
        Control = ControlKind.Number,
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

    private static FieldGroup Resolved(string key, params Field[] fields)
    {
        var group = new FieldGroup { Key = key };
        group.Fields.Add(fields);
        return group;
    }

    private static FieldGroupViewModel GroupOf(Func<string, bool>? onFloor, FieldGroup resolved)
    {
        var group = new FieldGroupViewModel((_, _) => { }, onFloor: onFloor);
        group.Apply(resolved, Vocabulary.Empty, null);
        return group;
    }

    [Fact]
    public void TheFloorNamesWhatAGuestTouches()
    {
        Assert.True(FloorLayout.OnFloor("publish.monitor"));
        Assert.True(FloorLayout.OnFloor("relay.group_key"));
        Assert.True(FloorLayout.OnFloor("relay.display_name"));

        Assert.False(FloorLayout.OnFloor("publish.fps"));
        Assert.False(FloorLayout.OnFloor("publish.format"));
        Assert.False(FloorLayout.OnFloor("publish.publish_transport"));
        Assert.False(FloorLayout.OnFloor("relay.tls"));
        Assert.False(FloorLayout.OnFloor("relay.srt_port"));
    }

    [Fact]
    public void AStepSplitsIntoFloorAndFold()
    {
        var group = GroupOf(FloorLayout.OnFloor, Resolved("relay",
            Text("relay.group_key"), Text("relay.display_name"), Text("relay.tls")));

        Assert.Equal(["relay.group_key", "relay.display_name"], group.Floor.Select(field => field.Key));
        Assert.Equal(["relay.tls"], group.Folded.Select(field => field.Key));
        Assert.Equal(3, group.Fields.Count);
        Assert.True(group.Fold.HasAny);
        Assert.Equal("1 option", group.Fold.Count);
    }

    /// <summary>The watch panel and every other caller without a floor keeps drawing everything.</summary>
    [Fact]
    public void NoPredicateKeepsEveryControlOnTheFloor()
    {
        var group = GroupOf(null, Resolved("relay",
            Text("relay.group_key"), Text("relay.tls")));

        Assert.Equal(2, group.Floor.Count);
        Assert.Empty(group.Folded);
        Assert.False(group.Fold.HasAny);
    }

    [Fact]
    public void AnOpenFoldSurvivesARenderPass()
    {
        var resolved = Resolved("relay", Text("relay.group_key"), Text("relay.tls"));
        var group = GroupOf(FloorLayout.OnFloor, resolved);

        group.Fold.Shown = true;
        group.Apply(resolved, Vocabulary.Empty, null);

        Assert.True(group.Fold.Shown);
        Assert.Equal(["relay.tls"], group.Folded.Select(field => field.Key));
    }

    [Fact]
    public void TheFoldToggleFlips()
    {
        var group = GroupOf(FloorLayout.OnFloor, Resolved("relay", Text("relay.tls")));

        group.Fold.ToggleCommand.Execute(null);
        Assert.True(group.Fold.Shown);

        group.Fold.ToggleCommand.Execute(null);
        Assert.False(group.Fold.Shown);
    }

    /// <summary>Every quality control sits behind the step's own fold, the presets standing in for them.</summary>
    [Fact]
    public void TheQualityStepFoldsWhole()
    {
        var group = GroupOf(null, Resolved("quality",
            Select("publish.format", "h264"), Number("publish.gop", 60)));
        var step = new QualityStepViewModel(group);

        Assert.False(step.Fold.Shown);
        Assert.True(step.Fold.HasAny);
        Assert.Equal("2 options", step.Fold.Count);
    }

    /// <summary>The source rows stay on the floor; the format under the list is what folds.</summary>
    [Fact]
    public void TheAudioFormatFoldsAlone()
    {
        var group = GroupOf(null, Resolved("audio",
            Select("publish.audio_sources[0].source", "none"),
            Select("publish.audio_codec", "opus")));
        var step = new AudioStepViewModel(group);

        Assert.False(step.Fold.Shown);
        Assert.True(step.Fold.HasAny);
        Assert.Equal("1 option", step.Fold.Count);
        Assert.Equal(["publish.audio_codec"], step.UnderList.Select(field => field.Key));
    }
}
