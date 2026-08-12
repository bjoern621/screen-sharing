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
/// Every figure the live screen shows is a measurement some other side took.
///
/// The screen was drawn from the mockups first, and for a while the numbers on it were the
/// mockup's own - a timer that read the same eight digits whatever was publishing, a window
/// label naming a span the plot did not cover, a rule marking a ceiling wherever the design
/// had put it. These tests state a reading and assert that what the screen says came out of
/// it, which is the property a seed cannot have.
/// </summary>
public sealed class BroadcastFiguresTests
{
    /// <summary>One encoder sample: a rate over the last interval, at a point in the run.</summary>
    private static PublishStats Sample(double mbps, double timeSec) => new()
    {
        InstMbps = mbps,
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

        strip.Show(Destination.Setup, broadcastAvailable: true, "00:00:07");

        Assert.True(strip.ShowsSharing);
        Assert.Equal("00:00:07", strip.SharingTimer);
    }

    [Fact]
    public void TheStripsPillGoesWithTheStreamItReported()
    {
        var strip = new NavStripViewModel(static _ => { });

        strip.Show(Destination.Setup, broadcastAvailable: true, "00:00:07");
        strip.Show(Destination.Setup, broadcastAvailable: false, Figure.NoValue);

        Assert.False(strip.ShowsSharing);
        Assert.Equal("", strip.SharingTimer);
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

        // Round trip and loss, with no relay snapshot to read a viewer's out of. They are the
        // relay's figures, so no snapshot is no measurement - and no measurement is an ellipsis.
        Assert.Equal(Figure.NoValue, bar.Figures[2].Value);
        Assert.Equal(Figure.NoValue, bar.Figures[3].Value);
    }

    /// <summary>
    /// The axis is a fixed span, so the label names it and does not grow with the run. It used to
    /// be the span the samples happened to cover, which meant a plot that read <c>3 s</c> a
    /// moment after sharing started and crept upwards from there.
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
    /// A moment sits at one place on the card whatever the run has reached: the newest sample is
    /// the right edge, and how far left a point sits is how long ago it was taken. A run younger
    /// than the window therefore fills the right of the plot and leaves the rest of it empty
    /// rather than being stretched across it.
    /// </summary>
    [Fact]
    public void APointIsPlacedByWhenItWasTakenAndNotByHowManyThereAre()
    {
        var young = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), Sample(4, 15), null),
            Samples = [Sample(3, 0), Sample(5, 5), Sample(4, 15)],
        };

        // Fifteen seconds of a sixty-second window: a quarter of the width, hard against the
        // right edge.
        Assert.Equal(3, young.Egress.Count);
        Assert.Equal(PlotSeries.Extent.Width, young.Egress[^1].X, 6);
        Assert.Equal(PlotSeries.Extent.Width * 0.75, young.Egress[0].X, 6);

        // A run past the window keeps the same scale and drops what fell off the back of it.
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
    /// A pipeline that dies and comes back leaves the stream live, so its samples are appended to
    /// the ones before it and its running clock starts again at zero. The plot draws the run the
    /// newest sample belongs to; laying both over one axis would put the old one off the right
    /// edge of the card.
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
        // A peak of 5 Mb/s against a 4 Mb/s ceiling: the curve's peak is 85% of the way up
        // from the floor, so the ceiling lands at 68% of it, which is 32% down from the top.
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

    [Fact]
    public void NothingDetectsCongestionSoNoBandIsNamed()
    {
        var plots = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), Sample(4, 12), null),
        };

        Assert.Equal("", plots.Band);
    }

    /// <summary>
    /// The nudge card promised a live-safe apply in its markup while its view model stated, in
    /// the same breath, that no such effect exists - and the reader saw the promise, because the
    /// markup drew the promise and bound neither the greying nor the reason.
    ///
    /// Both sentences come off one table now, so the test states the property that made the
    /// contradiction possible: the card's words and the card's behaviour agree.
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
    /// The configuration card used to divide its settings into ones needing a restart and ones
    /// that did not. Nothing has ever reached a running pipeline without restarting it, so the
    /// second half of that sentence described an effect that does not exist.
    /// </summary>
    [Fact]
    public void TheConfigurationCardSaysEverySettingRestartsTheStream()
    {
        var card = new ConfigCardViewModel();

        Assert.Contains("restarting it", card.ReadOnly);
        Assert.DoesNotContain("do not", card.ReadOnly);
    }

    /// <summary>
    /// The broadcast destination cannot be reached unless a stream is live, and stopping one
    /// takes the window off it. So "nothing is publishing" is the one thing an empty card here
    /// is never showing - it is showing a form resolve that has not answered yet, which is the
    /// ordinary first second of every broadcast.
    /// </summary>
    [Fact]
    public void AnUndescribedConfigurationSaysItIsBeingReadRatherThanThatNothingIsPublishing()
    {
        var card = new ConfigCardViewModel();

        Assert.False(card.HasRows);
        Assert.DoesNotContain("Nothing is publishing", card.Notice);
        Assert.Contains("Reading what the running stream", card.Notice);
    }
}
