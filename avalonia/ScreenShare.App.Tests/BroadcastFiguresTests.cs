using ScreenShare.Api.V1;
using ScreenShare.App.Features.Broadcast.ConfigCard.ViewModel;
using ScreenShare.App.Features.Broadcast.HeaderStats.ViewModel;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Features.Broadcast.Plots.Model;
using ScreenShare.App.Features.Broadcast.Plots.ViewModel;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.NavStrip.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Every figure on the live screen is a measurement some other side took.
///
/// A figure with nothing behind it looks measured: a timer reading the same eight digits whatever is publishing,
/// a window label naming a span the plot does not cover, a rule marking a ceiling wherever the design put it.
/// Each test states a reading and asserts the screen's figure came out of it, which a fixed number cannot do.
/// </summary>
public sealed class BroadcastFiguresTests
{
    /// <summary>Rate over the last interval, at a point in the run.</summary>
    private static PublishStats Sample(double mbps, double timeSec) => new()
    {
        InstMbps = mbps,
        TimeSec = timeSec,
    };

    /// <summary>Sample from a run that has been timed and has measured no rate.</summary>
    private static PublishStats Timed(double timeSec) => new()
    {
        TimeSec = timeSec,
    };

    /// <summary>
    /// Running stream, held to a rate where one is passed.
    /// The ceiling is the backend's reading of what bounds this encoder, not the settings field beside it:
    /// a quality target on an encoder that holds none is unbounded whatever the field carries.
    /// </summary>
    private static PublishState Live(int? ceilingMbps = null)
    {
        var live = new PublishState.Types.Live { Publish = new PublishSettings(), StreamName = "desk" };
        if (ceilingMbps is { } ceiling)
        {
            live.RateCeilingMbps = ceiling;
        }

        return new PublishState { Live = live };
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
    /// The screen reports the stream that has ended as well as the running one,
    /// so the segment that opens it is not a control the running state takes away.
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
        // Figures[0] is the encoder's rate, Figures[5] the relay's reader count.
        Assert.Equal("4.25", bar.Figures[0].Value);
        Assert.Equal("3", bar.Figures[5].Value);
    }

    /// <summary>
    /// A live publish and a snapshot name one stream, so a machine in a group reads its own figures:
    /// the prefix the group publishes under is the backend's and reaches neither string.
    /// </summary>
    [Fact]
    public void AGroupedPublishReadsItsOwnRelayRow()
    {
        var relay = new RelayStatus { Reachable = true };
        relay.Paths.Add(new RelayPath { Name = "bjoern/monitor-0", Readers = 2 });

        var live = new PublishState
        {
            Live = new PublishState.Types.Live { Publish = new PublishSettings(), StreamName = "bjoern/monitor-0" },
        };

        Assert.Equal(2, BroadcastSnapshot.Of(live, null, relay).Viewers);
    }

    [Fact]
    public void AFigureNothingMeasuresReadsAsAbsentRatherThanAsZero()
    {
        var bar = new HeaderStatsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), Sample(4.25, 12), null),
        };

        // Figures[3] and Figures[4] are round trip and loss, which only the relay measures.
        // No snapshot, no measurement, and no measurement reads as an ellipsis.
        Assert.Equal(Figure.NoValue, bar.Figures[3].Value);
        Assert.Equal(Figure.NoValue, bar.Figures[4].Value);
    }

    /// <summary>
    /// The axis is a fixed span, so the label names the axis, not the span the samples happen to cover:
    /// that reads <c>3 s</c> a moment after sharing starts and creeps upwards from there.
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
    /// A run younger than the window fills the right of the plot and leaves the rest empty,
    /// rather than stretching across it.
    /// </summary>
    [Fact]
    public void APointIsPlacedByWhenItWasTakenAndNotByHowManyThereAre()
    {
        var young = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), Sample(4, 15), null),
            Samples = [Sample(3, 0), Sample(5, 5), Sample(4, 15)],
        };

        // 15 s of a 60 s window: oldest point a quarter of the width in, newest hard against the right edge.
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
    /// A pipeline that dies and comes back leaves the stream live, so its samples follow the earlier ones
    /// and its clock starts again at zero.
    /// The plot draws the run the newest sample belongs to,
    /// one axis over both putting the earlier run off the right edge of the card.
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
    /// The axis is then on the new run's clock while every rate in the buffer is on the old one's,
    /// so the card waits for a shape on the new clock rather than plotting the old run against it.
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
        // Peak 5 Mb/s against a 4 Mb/s ceiling: the peak sits 85% up from the floor,
        // so the ceiling lands at 68% of that height, 32% down from the top.
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
    /// The relay marks no congestion interval, so the plot shades none and nothing names one:
    /// a caption would sit over a band the plot cannot draw, at a fraction of the width this side chose.
    /// </summary>
    [Fact]
    public void NothingDetectsCongestionSoNoBandIsNamed()
    {
        var reading = BroadcastSnapshot.Of(Live(), Sample(4, 12), null);

        Assert.Equal("", reading.CongestionAt);
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
    /// A resolve that has not answered is the ordinary first second of every broadcast,
    /// and a card reading a pipeline with nothing publishing would wait on an answer nothing asked for.
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

    /// <summary>Sample from a run that timed a frame through the encoder.</summary>
    private static PublishStats Encoded(double transitMs, double timeSec) => new()
    {
        TransitMs = transitMs,
        TimeSec = timeSec,
    };

    /// <summary>One reader on this stream's path, timed where the relay times the leg.</summary>
    private static RelayStatus Roster(double? rttMs)
    {
        var reader = new RelayReader { Transport = "srt" };
        if (rttMs is { } rtt)
        {
            reader.RttMs = rtt;
        }

        var path = new RelayPath { Name = "desk", Readers = 1 };
        path.ReaderRoster.Add(reader);

        var relay = new RelayStatus { Reachable = true };
        relay.Paths.Add(path);
        return relay;
    }

    /// <summary>
    /// One second is short enough for a healthy stream to measure nothing: an encoder that emitted no frame
    /// over the interval timed none, and the row would otherwise alternate between a figure and an ellipsis.
    /// </summary>
    [Fact]
    public void AFigureNoPassMeasuredReadsTheLastOneThatDid()
    {
        var held = new HeldFigures();

        held.Fill(BroadcastSnapshot.Of(Live(), Encoded(20.5, 12), null));
        var reading = held.Fill(BroadcastSnapshot.Of(Live(), Timed(13), null));

        Assert.Equal(20.5, reading.EncodeMs);
    }

    /// <summary>
    /// A stream that stopped measures nothing at all, and the next one is a different stream.
    /// A figure carried across either would name this stream by the last one's reading.
    /// </summary>
    [Fact]
    public void AStoppedStreamHoldsNothingForTheNextOne()
    {
        var held = new HeldFigures();
        held.Fill(BroadcastSnapshot.Of(Live(), Encoded(20.5, 12), null));

        var idle = held.Fill(BroadcastSnapshot.Of(new PublishState(), null, null));

        Assert.Null(idle.EncodeMs);

        var next = held.Fill(BroadcastSnapshot.Of(Live(), Timed(1), null));

        Assert.Null(next.EncodeMs);
    }

    /// <summary>
    /// A path naming readers and timing none of them is the relay's answer rather than a gap in it,
    /// and the header explains that absence in a sentence.
    /// A round trip held over it would name one nobody is taking.
    /// </summary>
    [Fact]
    public void ARelayThatTimesNoReaderStatesTheAbsence()
    {
        var held = new HeldFigures();
        held.Fill(BroadcastSnapshot.Of(Live(), Timed(12), Roster(rttMs: 42)));

        var reading = held.Fill(BroadcastSnapshot.Of(Live(), Timed(13), Roster(rttMs: null)));

        Assert.Null(reading.RttMs);
        Assert.Equal(1, reading.Viewers);
    }

    /// <summary>
    /// A poll that landed on nothing states no path, which says nothing about the viewers on it:
    /// same stream, and the readers on it were counted a second ago.
    /// </summary>
    [Fact]
    public void APollThatNamesNoPathHoldsWhatTheLastOneNamed()
    {
        var held = new HeldFigures();
        held.Fill(BroadcastSnapshot.Of(Live(), Timed(12), Roster(rttMs: 42)));

        var reading = held.Fill(BroadcastSnapshot.Of(Live(), Timed(13), null));

        Assert.Equal(1, reading.Viewers);
        Assert.Equal(42, reading.RttMs);
    }

    /// <summary>
    /// A stream between attempts has no pipeline measuring anything, and the pill says so.
    /// Figures left standing beside that would read as a stream carrying frames.
    /// </summary>
    [Fact]
    public void AStreamWaitingOutARetryHoldsNoFigure()
    {
        var held = new HeldFigures();
        held.Fill(BroadcastSnapshot.Of(Live(), Encoded(20.5, 12), null));

        var live = Live();
        live.Live.Retry = new PublishState.Types.Retry { Attempt = 1, Budget = 3 };
        var reading = held.Fill(BroadcastSnapshot.Of(live, Timed(13), null));

        Assert.Null(reading.EncodeMs);
    }

    /// <summary>
    /// A quality target the encoder holds free of the rate gets no rule and no label:
    /// a height marked on a plot reads as a bound something is holding.
    /// </summary>
    [Fact]
    public void AnUnboundedEncodeIsMarkedByNoCeiling()
    {
        var plots = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), Sample(5, 20), null),
            Samples = [Sample(3, 10), Sample(5, 20)],
        };

        Assert.True(plots.HasEgress);
        Assert.False(plots.HasCeiling);
        Assert.Equal("", plots.Ceiling);
    }
}
