using ScreenShare.Api.V1;
using ScreenShare.App.Features.Broadcast.ConfigCard.ViewModel;
using ScreenShare.App.Features.Broadcast.HeaderStats.ViewModel;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Features.Broadcast.Nudge.ViewModel;
using ScreenShare.App.Features.Broadcast.Plots.Model;
using ScreenShare.App.Features.Broadcast.Plots.ViewModel;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.NavStrip.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Every figure on the live screen is a measurement some other side took.
///
/// The screen was drawn from the mockups first, and its numbers were the mockup's own: a timer reading the
/// same eight digits whatever was publishing, a window label naming a span the plot did not cover, a rule
/// marking a ceiling wherever the design had put it.
/// Each test states a reading and asserts that the screen's figure came out of it, which a seeded number
/// cannot do.
/// </summary>
public sealed class BroadcastFiguresTests
{
    /// <summary>A rate over the last interval, at a point in the run.</summary>
    private static PublishStats Sample(double mbps, double timeSec) => new()
    {
        InstMbps = mbps,
        TimeSec = timeSec,
    };

    /// <summary>A sample from a run that has been timed and not yet measured a rate.</summary>
    private static PublishStats Timed(double timeSec) => new()
    {
        TimeSec = timeSec,
    };

    private static PublishState Live(int? ceilingMbps = null)
    {
        var settings = new PublishSettings { Name = "desk" };
        if (ceilingMbps is { } ceiling)
        {
            settings.MaxrateMbps = ceiling;
        }

        return new PublishState { Live = new PublishState.Types.Live { Publish = settings } };
    }

    [Fact]
    public void TheSharingTimerIsTheEncodersOwnClock()
    {
        var reading = BroadcastSnapshot.Of(Live(), Sample(4, 3_671), null);

        Assert.Equal("01:01:11", reading.Elapsed);
    }

    [Fact]
    public void TheSharingTimerIsUnmeasuredBeforeTheFirstSample()
    {
        var reading = BroadcastSnapshot.Of(Live(), null, null);

        Assert.True(reading.IsLive);
        Assert.Equal(Figure.NoValue, reading.Elapsed);
    }

    [Fact]
    public void TheStripsPillShowsTheTimerItWasTold()
    {
        var strip = new NavStripViewModel(static _ => { });

        strip.Show(Destination.Setup, sharing: true, "00:00:07");

        Assert.True(strip.ShowsSharing);
        Assert.Equal("00:00:07", strip.SharingTimer);
    }

    [Fact]
    public void TheStripsPillGoesWithTheStreamItReported()
    {
        var strip = new NavStripViewModel(static _ => { });

        strip.Show(Destination.Setup, sharing: true, "00:00:07");
        strip.Show(Destination.Setup, sharing: false, Figure.NoValue);

        Assert.False(strip.ShowsSharing);
        Assert.Equal("", strip.SharingTimer);
    }

    /// <summary>
    /// The screen reports the stream that has ended as well as the running one, so the segment that opens it
    /// is not a control the running state takes away.
    /// </summary>
    [Fact]
    public void TheStripReachesBroadcastWithNothingPublishing()
    {
        var asked = new List<Destination>();
        var strip = new NavStripViewModel(asked.Add);

        strip.Show(Destination.Setup, sharing: false, Figure.NoValue);
        strip.SelectedTab = strip.Tabs.Single(tab => tab.Value == Destination.Broadcast);

        Assert.Equal(Destination.Broadcast, Assert.Single(asked));
    }

    [Fact]
    public void TheHeaderFiguresAreTheSamplesAndTheRelays()
    {
        var relay = new RelayStatus { Reachable = true };
        relay.Paths.Add(new RelayPath { Name = "desk", Readers = 3 });

        var bar = new HeaderStatsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), Sample(4.25, 12), relay),
        };

        Assert.True(bar.IsSharing);
        Assert.Equal("00:00:12", bar.Elapsed);
        // Figures[0] is the encoder's rate, Figures[4] the relay's reader count.
        Assert.Equal("4.25", bar.Figures[0].Value);
        Assert.Equal("3", bar.Figures[4].Value);
    }

    [Fact]
    public void AFigureNothingMeasuresReadsAsAbsentRatherThanAsZero()
    {
        var bar = new HeaderStatsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), Sample(4.25, 12), null),
        };

        // Figures[2] and Figures[3] are round trip and loss, which only the relay measures.
        // No snapshot is therefore no measurement, and no measurement is an ellipsis.
        Assert.Equal(Figure.NoValue, bar.Figures[2].Value);
        Assert.Equal(Figure.NoValue, bar.Figures[3].Value);
    }

    /// <summary>
    /// The axis is a fixed span, so the label names the axis rather than the span the samples happen to
    /// cover, which read <c>3 s</c> a moment after sharing started and crept upwards from there.
    /// </summary>
    [Fact]
    public void ThePlotStatesTheWindowItCoversWhateverTheRunHasReached()
    {
        var young = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), Sample(4, 30), null),
            Samples = [Sample(3, 10), Sample(5, 20), Sample(4, 30)],
        };

        Assert.True(young.HasEgress);
        Assert.Equal("60 s", young.Window);

        var old = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), Sample(4, 400), null),
            Samples = Enumerable.Range(0, 400).Select(second => Sample(4, second)).ToList(),
        };

        Assert.Equal("60 s", old.Window);
    }

    /// <summary>
    /// The newest sample is the right edge, and how far left a point sits is how long ago it was taken.
    /// A run younger than the window therefore fills the right of the plot and leaves the rest empty rather
    /// than being stretched across it.
    /// </summary>
    [Fact]
    public void APointIsPlacedByWhenItWasTakenAndNotByHowManyThereAre()
    {
        var young = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), Sample(4, 15), null),
            Samples = [Sample(3, 0), Sample(5, 5), Sample(4, 15)],
        };

        // 15 s of a 60 s window: the oldest point a quarter of the width in, the newest hard against the
        // right edge.
        Assert.Equal(3, young.Egress.Count);
        Assert.Equal(PlotSeries.Extent.Width, young.Egress[^1].X, 6);
        Assert.Equal(PlotSeries.Extent.Width * 0.75, young.Egress[0].X, 6);

        // Past the window the scale holds, and what fell off the back is dropped.
        var running = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), Sample(4, 400), null),
            Samples = Enumerable.Range(0, 401).Select(second => Sample(4, second)).ToList(),
        };

        Assert.Equal(61, running.Egress.Count);
        Assert.Equal(0, running.Egress[0].X, 6);
        Assert.Equal(PlotSeries.Extent.Width, running.Egress[^1].X, 6);
    }

    /// <summary>
    /// A pipeline that dies and comes back leaves the stream live, so its samples are appended to the earlier
    /// ones and its clock starts again at zero.
    /// The plot draws the run the newest sample belongs to, since one axis over both would put the earlier
    /// run off the right edge of the card.
    /// </summary>
    [Fact]
    public void ARelaunchedPipelineIsNotDrawnOverTheOneBeforeIt()
    {
        var plots = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), Sample(4, 2), null),
            Samples = [Sample(3, 100), Sample(5, 101), Sample(3, 0), Sample(5, 1), Sample(4, 2)],
        };

        Assert.Equal(3, plots.Egress.Count);
        Assert.Equal(PlotSeries.Extent.Width, plots.Egress[^1].X, 6);
    }

    /// <summary>
    /// A rate is measured over the last interval, so the first sample of a run carries a time and no rate.
    /// The axis is then on the new run's clock while every rate in the buffer is on the old one's, and the
    /// card waits for the new run to have a shape rather than placing the old one against a clock that never
    /// counted it.
    /// </summary>
    [Fact]
    public void ARunThatHasBeenTimedAndNotYetMeasuredDrawsNothing()
    {
        var plots = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), Timed(0), null),
            Samples = [Sample(3, 100), Sample(5, 101), Timed(0)],
        };

        Assert.Empty(plots.Egress);
        Assert.False(plots.HasEgress);
        Assert.True(double.IsNaN(plots.CeilingFraction));
    }

    [Fact]
    public void APlotWithNoCurveStatesNoWindow()
    {
        var plots = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), null, null),
        };

        Assert.False(plots.HasEgress);
        Assert.Equal("", plots.Window);
    }

    [Fact]
    public void TheCeilingRuleSitsWhereTheCeilingFallsOnTheCurve()
    {
        // Peak 5 Mb/s against a 4 Mb/s ceiling: the peak sits 85% up from the floor, so the ceiling lands at
        // 68% of that height, 32% down from the top.
        var plots = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(ceilingMbps: 4), Sample(5, 20), null),
            Samples = [Sample(3, 10), Sample(5, 20)],
        };

        Assert.Equal(0.32, plots.CeilingFraction, 3);
        Assert.Equal("vbv ceiling 4 Mb/s", plots.Ceiling);
    }

    [Fact]
    public void ACeilingTheStreamIsNowhereNearIsMarkedByNoRule()
    {
        var plots = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(ceilingMbps: 20), Sample(5, 20), null),
            Samples = [Sample(3, 10), Sample(5, 20)],
        };

        Assert.True(double.IsNaN(plots.CeilingFraction));
        Assert.Equal("vbv ceiling 20 Mb/s", plots.Ceiling);
    }

    /// <summary>
    /// The relay marks no congestion interval, so the plot shades none and nothing names one: a caption
    /// would be placed over a band the plot cannot draw, at a fraction of the width this side chose.
    /// </summary>
    [Fact]
    public void NothingDetectsCongestionSoNoBandIsNamed()
    {
        var reading = BroadcastSnapshot.Of(Live(), Sample(4, 12), null);

        Assert.Equal("", reading.CongestionAt);
    }

    /// <summary>
    /// The markup promised a live-safe apply while the view model stated that no such effect exists, and the
    /// reader saw the promise because the markup bound neither the greying nor the reason.
    /// Both sentences come off one table, so the card's words and the card's behaviour agree.
    /// </summary>
    [Fact]
    public void TheNudgeCardNeverPromisesAnApplyTheBackendHasNoEffectFor()
    {
        var nudge = new NudgeViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), Sample(4, 12), null),
        };

        Assert.False(nudge.IsEnabled);
        Assert.NotEqual("", nudge.Reason);
        Assert.NotEqual("", nudge.Caveat);
        Assert.DoesNotContain("without a reconnect", nudge.Caveat);
        Assert.Contains("restarts the stream", nudge.Reason);
    }

    /// <summary>
    /// Nothing reaches a running pipeline without restarting it, so a card dividing its settings into ones
    /// needing a restart and ones not needing one describes an effect that does not exist.
    /// </summary>
    [Fact]
    public void TheConfigurationCardSaysEverySettingRestartsTheStream()
    {
        var card = new ConfigCardViewModel();

        Assert.Contains("restarting it", card.ReadOnly);
        Assert.DoesNotContain("do not", card.ReadOnly);
    }

    /// <summary>
    /// The card is empty for two different reasons, and the destination is reachable in both.
    /// A resolve that has not answered is the ordinary first second of every broadcast, and a card saying it
    /// is reading a pipeline with nothing publishing would wait on an answer nothing asked for.
    /// </summary>
    [Fact]
    public void AnUndescribedConfigurationSaysWhichAbsenceItIs()
    {
        var live = new ConfigCardViewModel { IsLive = true };

        Assert.False(live.HasRows);
        Assert.Contains("Reading what the running stream", live.Notice);

        var idle = new ConfigCardViewModel();

        Assert.False(idle.HasRows);
        Assert.Contains("Nothing is publishing", idle.Notice);
    }
}
