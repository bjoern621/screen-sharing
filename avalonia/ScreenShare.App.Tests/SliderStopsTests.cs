using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Fields.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Where a swept control comes to rest.
///
/// A slider stops on the round figures its step names and on the two ends of its range, so a floor the step
/// steps over stays reachable: 20 ms under a 50 ms step is offered 20, 50, 100 rather than 20, 70, 120.
/// The widget stops on both ends whatever it is handed, so the list carries neither.
/// </summary>
public sealed class SliderStopsTests
{
    private static FieldViewModel Swept(long min, long max, long step)
    {
        var field = new FieldViewModel("viewer.srt_watch_latency_ms", (_, _) => { });
        field.Apply(
            new Field
            {
                Key = "viewer.srt_watch_latency_ms",
                Control = ControlKind.Slider,
                Visible = true,
                Enabled = true,
                Range = new NumericRange { Min = min, Max = max, Step = step },
                Value = new FieldValue { Number = min },
            },
            Vocabulary.Empty);
        return field;
    }

    [Fact]
    public void TheStopsAreTheRoundFiguresInsideTheRange()
    {
        var stops = Swept(20, 8000, 50).Ticks;

        Assert.Equal(50, stops[0]);
        Assert.Equal(100, stops[1]);
        Assert.Equal(7950, stops[^1]);
        Assert.All(stops, stop => Assert.Equal(0, stop % 50));
        Assert.All(stops, stop => Assert.InRange(stop, 21, 7999));
    }

    /// <summary>
    /// A floor and a ceiling the step never lands on are the pair a sweep must still reach, the floor being
    /// the shortest window there is.
    /// </summary>
    [Fact]
    public void TheEndsAreLeftToTheWidget()
    {
        var stops = Swept(20, 7990, 50).Ticks;

        Assert.DoesNotContain(20d, stops);
        Assert.DoesNotContain(7990d, stops);
    }

    /// <summary>A fresh list per pass would rebind the control under a hand resting on the thumb.</summary>
    [Fact]
    public void ASecondPassLeavesTheStopsWhereTheyAre()
    {
        var field = Swept(20, 8000, 50);
        var stops = field.Ticks;
        var first = stops[0];
        var count = stops.Count;

        field.Apply(
            new Field
            {
                Key = "viewer.srt_watch_latency_ms",
                Control = ControlKind.Slider,
                Visible = true,
                Enabled = true,
                Range = new NumericRange { Min = 20, Max = 8000, Step = 50 },
                Value = new FieldValue { Number = 300 },
            },
            Vocabulary.Empty);

        Assert.Same(stops, field.Ticks);
        Assert.Equal(count, field.Ticks.Count);
        Assert.Equal(first, field.Ticks[0]);
    }

    /// <summary>A range narrowing under the codec moves the stops with it.</summary>
    [Fact]
    public void ANarrowedRangeDropsTheStopsPastItsEnd()
    {
        var field = Swept(0, 51, 1);
        Assert.Equal(50, field.Ticks.Count);

        field.Apply(
            new Field
            {
                Key = "viewer.srt_watch_latency_ms",
                Control = ControlKind.Slider,
                Visible = true,
                Enabled = true,
                Range = new NumericRange { Min = 0, Max = 12, Step = 1 },
                Value = new FieldValue { Number = 6 },
            },
            Vocabulary.Empty);

        Assert.Equal(11, field.Ticks.Count);
        Assert.Equal(11, field.Ticks[^1]);
    }

    /// <summary>
    /// A control that is not swept states no ends, so building stops for one would count off the widest
    /// number there is.
    /// </summary>
    [Fact]
    public void ATypedControlCarriesNoStops()
    {
        var field = new FieldViewModel("publish.bitrate_mbps", (_, _) => { });
        field.Apply(
            new Field
            {
                Key = "publish.bitrate_mbps",
                Control = ControlKind.Number,
                Visible = true,
                Enabled = true,
                Value = new FieldValue { Number = 20 },
            },
            Vocabulary.Empty);

        Assert.Empty(field.Ticks);
    }
}
