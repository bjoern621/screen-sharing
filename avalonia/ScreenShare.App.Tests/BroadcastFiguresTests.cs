using ScreenShare.Api.V1;
using ScreenShare.App.Features.Broadcast.HeaderStats.ViewModel;
using ScreenShare.App.Features.Broadcast.Model;
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
    public void TheOnAirTimerIsTheEncodersOwnClock()
    {
        var reading = BroadcastSnapshot.Of(Live(), Sample(4, 3_671), null);

        Assert.Equal("01:01:11", reading.Elapsed);
    }

    [Fact]
    public void TheOnAirTimerIsUnmeasuredBeforeTheFirstSample()
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

        Assert.True(strip.ShowsOnAir);
        Assert.Equal("00:00:07", strip.OnAirTimer);
    }

    [Fact]
    public void TheStripsPillGoesWithTheStreamItReported()
    {
        var strip = new NavStripViewModel(static _ => { });

        strip.Show(Destination.Setup, broadcastAvailable: true, "00:00:07");
        strip.Show(Destination.Setup, broadcastAvailable: false, Figure.NoValue);

        Assert.False(strip.ShowsOnAir);
        Assert.Equal("", strip.OnAirTimer);
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

        Assert.True(bar.IsOnAir);
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

        // Round trip and loss, which nothing in the pipeline reports.
        Assert.Equal(Figure.NoValue, bar.Figures[2].Value);
        Assert.Equal(Figure.NoValue, bar.Figures[3].Value);
    }

    [Fact]
    public void ThePlotStatesTheWindowItsSamplesCover()
    {
        var plots = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), Sample(4, 30), null),
            Samples = [Sample(3, 10), Sample(5, 20), Sample(4, 30)],
        };

        Assert.True(plots.HasEgress);
        Assert.Equal("20 s", plots.Window);
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
}
