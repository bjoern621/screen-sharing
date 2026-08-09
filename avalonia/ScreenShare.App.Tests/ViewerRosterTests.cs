using System.Collections.Specialized;
using ScreenShare.Api.V1;
using ScreenShare.App.Features.Broadcast.HeaderStats.ViewModel;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Features.Broadcast.Plots.ViewModel;
using ScreenShare.App.Features.Broadcast.ViewerTable.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The viewer table is a row per reader the relay named, and every cell in it is a figure the
/// relay stated about that reader.
///
/// It used to be a count and a sentence saying that the relay reported how many were connected
/// and not who they were. That was true of what the backend read rather than of the relay: the
/// path list carries a reader array, and the per-protocol connection lists carry the figures.
/// These tests state a roster and assert that the rows came out of it - and, just as hard, that
/// a figure the relay did not state comes out absent rather than as a zero, because a viewer
/// nobody timed and a viewer with a perfect link are the two things this table may never
/// confuse.
/// </summary>
public sealed class ViewerRosterTests
{
    /// <summary>An SRT reader: the one leg the relay times a round trip and states a loss rate on.</summary>
    private static RelayReader Srt(string address, double rttMs, double lossPercent, ulong dropped = 0) => new()
    {
        Type = "srtConn",
        Id = $"id-{address}",
        Transport = "srt",
        RemoteAddr = address,
        Joined = "2026-08-09T22:01:12.0000000+00:00",
        BytesSent = 373004,
        RttMs = rttMs,
        LossPercent = lossPercent,
        PacketsSent = 385,
        PacketsLost = 0,
        PacketsDropped = dropped,
        FramesDiscarded = 0,
    };

    /// <summary>An RTMP reader: bytes and the relay's own discards, and nothing about the line.</summary>
    private static RelayReader Rtmp(string address, ulong discarded = 0) => new()
    {
        Type = "rtmpConn",
        Id = $"id-{address}",
        Transport = "rtmp",
        RemoteAddr = address,
        Joined = "2026-08-09T22:01:15.0000000+00:00",
        BytesSent = 358672,
        FramesDiscarded = discarded,
    };

    private static RelayStatus Serving(params RelayReader[] readers)
    {
        var path = new RelayPath { Name = "desk", Ready = true, Readers = readers.Length };
        path.ReaderRoster.AddRange(readers);

        var relay = new RelayStatus { Reachable = true };
        relay.Paths.Add(path);
        return relay;
    }

    private static PublishState Live() => new()
    {
        Live = new PublishState.Types.Live { Publish = new PublishSettings { Name = "desk" } },
    };

    private static ViewerTableViewModel Table(RelayStatus? relay)
    {
        var path = BroadcastSnapshot.PathOf(relay, "desk");
        var table = new ViewerTableViewModel
        {
            Reported = path is null ? [] : path.ReaderRoster.Select(ViewerRow.Of).ToList(),
            Readers = path?.Readers,
        };

        return table;
    }

    [Fact]
    public void ARowIsBuiltFromTheReaderTheRelayNamed()
    {
        var table = Table(Serving(Srt("10.0.0.4:52157", rttMs: 18.4, lossPercent: 0.25)));

        Assert.True(table.HasRows);
        var row = Assert.Single(table.Rows);

        Assert.Equal("10.0.0.4:52157", row.Name);
        Assert.Equal("18", row.Rtt);
        Assert.Equal("0.25", row.Loss);
        Assert.Equal("srt", row.Via);
        Assert.Equal("0", row.Dropped);
        Assert.NotEqual(Figure.NoValue, row.Joined);
    }

    [Fact]
    public void AFigureTheLegDoesNotReportReadsAsAbsentRatherThanAsZero()
    {
        var table = Table(Serving(Rtmp("10.0.0.9:55372")));

        var row = Assert.Single(table.Rows);

        // Nothing times an RTMP reader and nothing states its loss, so both read as unmeasured.
        Assert.Equal(Figure.NoValue, row.Rtt);
        Assert.Equal(Figure.NoValue, row.Loss);

        // What it does count is a measurement, and a measured zero prints as one.
        Assert.Equal("0", row.Dropped);
        Assert.Equal("rtmp", row.Via);
        Assert.False(row.IsStruggling);
    }

    [Fact]
    public void AReaderTheRelayDescribedNowhereIsStillNamed()
    {
        // Every figure absent: the path named this reader and the per-protocol list did not
        // answer, which is what a relay with that listener switched off produces.
        var unmeasured = new RelayReader { Type = "moqSession", Id = "5c1f", Transport = "moq" };

        var table = Table(Serving(unmeasured));

        var row = Assert.Single(table.Rows);
        Assert.Equal("5c1f", row.Name);
        Assert.Equal(Figure.NoValue, row.Joined);
        Assert.Equal(Figure.NoValue, row.Rtt);
        Assert.Equal(Figure.NoValue, row.Loss);
        Assert.Equal(Figure.NoValue, row.Dropped);
        Assert.Equal("moq", row.Via);
        Assert.False(row.IsStruggling);
    }

    /// <summary>
    /// A relay that named a reader nothing at all is an environment condition, not a bug in this
    /// code. The row renders as an unnameable viewer, which still says somebody is connected.
    /// </summary>
    [Fact]
    public void AReaderTheRelayNamedNothingStillRenders()
    {
        var table = Table(Serving(new RelayReader()));

        var row = Assert.Single(table.Rows);
        Assert.Equal(Figure.NoValue, row.Name);
        Assert.Equal(Figure.NoValue, row.Via);
        Assert.False(row.IsStruggling);
        Assert.Equal(1, table.Readers);
    }

    [Fact]
    public void OnlyAMeasuredFigureOverItsLimitMakesARowStruggle()
    {
        var table = Table(Serving(
            Srt("10.0.0.1:1", rttMs: 20, lossPercent: 0.1),      // healthy
            Srt("10.0.0.2:2", rttMs: 20, lossPercent: 7.5),      // loss over its limit
            Srt("10.0.0.3:3", rttMs: 480, lossPercent: 0),       // round trip over its limit
            Srt("10.0.0.4:4", rttMs: 20, lossPercent: 0, 12),    // dropped anything at all
            Rtmp("10.0.0.5:5")));                                // untimed, and so never struggling

        Assert.Equal(5, table.Rows.Count);
        Assert.Equal(3, table.StrugglingCount);
        Assert.False(table.Rows[0].IsStruggling);
        Assert.True(table.Rows[1].IsStruggling);
        Assert.True(table.Rows[2].IsStruggling);
        Assert.True(table.Rows[3].IsStruggling);
        Assert.False(table.Rows[4].IsStruggling);
    }

    [Fact]
    public void OneRowEndsTheTableAndCarriesNoSeparator()
    {
        var table = Table(Serving(Srt("10.0.0.1:1", 20, 0), Srt("10.0.0.2:2", 20, 0), Rtmp("10.0.0.3:3")));

        Assert.Equal(3, table.Rows.Count);
        Assert.False(table.Rows[0].IsLast);
        Assert.False(table.Rows[1].IsLast);
        Assert.True(table.Rows[2].IsLast);
    }

    /// <summary>
    /// The property the whole table hangs on: a roster pushed again unchanged does not repaint.
    /// The relay is polled while the reader has the pointer over the card, and a clear-and-fill
    /// on every poll would reset the scroll position of a table nothing had happened to.
    /// </summary>
    [Fact]
    public void AnUnchangedRosterRenderedAgainTouchesNothing()
    {
        var relay = Serving(Srt("10.0.0.1:1", 20, 0.5), Rtmp("10.0.0.2:2"));
        var table = Table(relay);

        var changes = 0;
        ((INotifyCollectionChanged)table.Rows).CollectionChanged += (_, _) => changes++;

        // The same roster read a second time, through the same path the render pass reads it by.
        table.Reported = BroadcastSnapshot.PathOf(relay, "desk")!.ReaderRoster.Select(ViewerRow.Of).ToList();
        table.Apply();
        table.Apply();

        Assert.Equal(0, changes);
        Assert.Equal(2, table.Rows.Count);
    }

    [Fact]
    public void ATableWithNoRowsSaysWhichAbsenceItIs()
    {
        var unasked = Table(null);
        Assert.False(unasked.HasRows);
        Assert.Contains("has not been asked", unasked.Notice);

        var watched = Table(Serving());
        Assert.False(watched.HasRows);
        Assert.Contains("Nobody is connected", watched.Notice);
    }

    /// <summary>
    /// The header's round trip and loss are one viewer's and the worst one's. A mean would be a
    /// figure nobody is experiencing, and would average a single struggling viewer away.
    /// </summary>
    [Fact]
    public void TheHeaderPromotesTheWorstViewersFigures()
    {
        var relay = Serving(
            Srt("10.0.0.1:1", rttMs: 12, lossPercent: 0.1),
            Srt("10.0.0.2:2", rttMs: 240, lossPercent: 4.5),
            Rtmp("10.0.0.3:3"));

        var reading = BroadcastSnapshot.Of(Live(), null, relay);

        Assert.Equal(240, reading.RttMs);
        Assert.Equal(4.5, reading.LossPercent);
        Assert.Equal(3, reading.Viewers);

        var bar = new HeaderStatsViewModel { Snapshot = reading };
        Assert.Equal("240", bar.Figures[2].Value);
        Assert.Equal("4.50", bar.Figures[3].Value);
        Assert.Contains("worst", bar.Figures[2].Unit);
        Assert.Contains("worst", bar.Figures[3].Unit);
    }

    /// <summary>
    /// A stream whose viewers are all on legs the relay does not time has viewers and no
    /// latency. That is not the same as a stream nobody is watching, and the header says so by
    /// showing a viewer count beside two absences.
    /// </summary>
    [Fact]
    public void ViewersOnAnUntimedLegLeaveTheLatencyFiguresAbsent()
    {
        var reading = BroadcastSnapshot.Of(Live(), null, Serving(Rtmp("10.0.0.3:3"), Rtmp("10.0.0.4:4")));

        Assert.Equal(2, reading.Viewers);
        Assert.Null(reading.RttMs);
        Assert.Null(reading.LossPercent);
    }

    /// <summary>
    /// The latency plot is the relay snapshots' own shape, the same way the egress plot is the
    /// encoder samples'. Two snapshots make a curve; one is a reading and not a shape.
    /// </summary>
    [Fact]
    public void TheLatencyCurvesAreDrawnFromTheRelaySnapshots()
    {
        var samples = new[]
        {
            Serving(Srt("10.0.0.1:1", rttMs: 20, lossPercent: 0)),
            Serving(Srt("10.0.0.1:1", rttMs: 60, lossPercent: 1.5)),
            Serving(Srt("10.0.0.1:1", rttMs: 40, lossPercent: 0.5)),
        };

        var plots = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), null, samples[^1]),
            RelaySamples = samples,
        };

        Assert.True(plots.HasLatency);
        Assert.Equal(3, plots.Rtt.Count);
        Assert.Equal(3, plots.Loss.Count);
        Assert.Equal("", plots.LatencyNotice);

        // Y grows downward, so the highest round trip is the smallest Y of the three.
        Assert.True(plots.Rtt[1].Y < plots.Rtt[0].Y);
        Assert.True(plots.Rtt[1].Y < plots.Rtt[2].Y);
    }

    [Fact]
    public void ALatencyPlotWithNothingToDrawSaysWhichAbsenceItIs()
    {
        var idle = new PlotsViewModel { Snapshot = BroadcastSnapshot.Of(null, null, null) };
        Assert.False(idle.HasLatency);
        Assert.Equal("nothing is publishing", idle.LatencyNotice);

        var unwatched = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), null, Serving()),
            RelaySamples = [Serving()],
        };
        Assert.False(unwatched.HasLatency);
        Assert.Equal("nobody is watching yet", unwatched.LatencyNotice);

        var untimed = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), null, Serving(Rtmp("10.0.0.3:3"))),
            RelaySamples = [Serving(Rtmp("10.0.0.3:3"))],
        };
        Assert.False(untimed.HasLatency);
        Assert.Equal("no viewer is on a leg the relay times", untimed.LatencyNotice);

        // Timed once. There is a measurement and no shape yet, and saying that nobody is timed
        // here would contradict the figure the header is showing from the same snapshot.
        var once = Serving(Srt("10.0.0.1:1", rttMs: 20, lossPercent: 0));
        var starting = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), null, once),
            RelaySamples = [once],
        };
        Assert.False(starting.HasLatency);
        Assert.Equal("waiting for the relay's next snapshot", starting.LatencyNotice);
        Assert.Equal(20, starting.Snapshot.RttMs);
    }
}
