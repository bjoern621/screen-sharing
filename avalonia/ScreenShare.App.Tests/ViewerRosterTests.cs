using System.Collections.Specialized;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Broadcast.HeaderStats.ViewModel;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Features.Broadcast.Plots.ViewModel;
using ScreenShare.App.Features.Broadcast.ViewerTable.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A row per reader the relay named, and a cell per figure it stated about that reader.
/// A figure the relay did not state reads as absent and never as a zero: a viewer nobody timed and a viewer
/// with a perfect link are the two this table may not confuse.
/// </summary>
public sealed class ViewerRosterTests
{
    /// <summary>SRT reader: the one leg the relay times and states a loss rate on.</summary>
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

    /// <summary>RTMP reader: bytes and the relay's own discards, nothing about the line.</summary>
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

    /// <summary>
    /// One snapshot as the session holds it: the relay's answer, and when this shell took it.
    /// The stamp is the only clock a departure has.
    /// </summary>
    private static RelayReading Reading(RelayStatus status, int second = 0)
        => new(status, new DateTimeOffset(2026, 8, 9, 22, 2, 0, TimeSpan.Zero).AddSeconds(second));

    private static RelayStatus Serving(params RelayReader[] readers)
    {
        var path = new RelayPath { Name = "desk", OwnName = "desk", Ready = true, Readers = readers.Length };
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
            IsLive = true,
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

        // Nothing times an RTMP reader and nothing states its loss.
        Assert.Equal(Figure.NoValue, row.Rtt);
        Assert.Equal(Figure.NoValue, row.Loss);

        // A measured zero is a measurement, and prints as one.
        Assert.Equal("0", row.Dropped);
        Assert.Equal("rtmp", row.Via);
        Assert.False(row.IsStruggling);
    }

    [Fact]
    public void AReaderTheRelayDescribedNowhereIsStillNamed()
    {
        // The path named this reader and no per-protocol list answered: a relay with that listener off.
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
    /// A relay that named a reader nothing is an Umgebungsfehler, so the row renders unnameable rather than
    /// asserting: somebody is connected either way.
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
            Srt("10.0.0.2:2", rttMs: 20, lossPercent: 7.5),      // loss past its limit
            Srt("10.0.0.3:3", rttMs: 480, lossPercent: 0),       // round trip past its limit
            Srt("10.0.0.4:4", rttMs: 20, lossPercent: 0, 12),    // dropped at all
            Rtmp("10.0.0.5:5")));                                // untimed, so never struggling

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
    /// The relay is polled while the pointer is over the card, and a clear-and-fill on every poll would reset
    /// the scroll position of a table nothing happened to.
    /// </summary>
    [Fact]
    public void AnUnchangedRosterRenderedAgainTouchesNothing()
    {
        var relay = Serving(Srt("10.0.0.1:1", 20, 0.5), Rtmp("10.0.0.2:2"));
        var table = Table(relay);

        var changes = 0;
        ((INotifyCollectionChanged)table.Rows).CollectionChanged += (_, _) => changes++;

        // The same roster again, through the path the render pass reads it by.
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

        // The card is drawn with nothing publishing too, where an empty roster is neither of the above.
        var idle = new ViewerTableViewModel();
        Assert.False(idle.HasRows);
        Assert.Contains("Nothing is publishing", idle.Notice);
    }

    /// <summary>
    /// The header's round trip and loss are the worst viewer's own.
    /// A mean is a figure nobody is experiencing, and averages a single struggling viewer away.
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
        Assert.Equal("240", bar.Figures[3].Value);
        Assert.Equal("4.50", bar.Figures[4].Value);
        Assert.Contains("worst", bar.Figures[3].Unit);
        Assert.Contains("worst", bar.Figures[4].Unit);
    }

    /// <summary>
    /// Viewers on legs nothing times are not a stream nobody is watching, and the header says which by
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
    /// Distinct and in the order the roster names them, since the value is read as prose.
    /// A reader the relay named no protocol for takes no part, so the sentence has no gap in it.
    /// </summary>
    [Fact]
    public void TheLegsAreTheDistinctTransportsOnTheRoster()
    {
        Assert.Equal("", BroadcastSnapshot.Of(Live(), null, Serving()).Legs);

        var oneLeg = BroadcastSnapshot.Of(Live(), null, Serving(Rtmp("10.0.0.3:3"), Rtmp("10.0.0.4:4")));
        Assert.Equal("rtmp", oneLeg.Legs);

        var two = BroadcastSnapshot.Of(Live(), null, Serving(Srt("10.0.0.1:1", 20, 0), Rtmp("10.0.0.3:3")));
        Assert.Equal("srt, rtmp", two.Legs);

        var unnamed = new RelayReader { Type = "somethingNew", Id = "id-1" };
        Assert.Equal("", BroadcastSnapshot.Of(Live(), null, Serving(unnamed)).Legs);
    }

    /// <summary>
    /// An unmeasured promoted figure carries why, where the reason is one a publisher can act on: viewers,
    /// none of them on a leg the relay times.
    /// A measured figure carries none.
    /// Neither does a stream nobody is watching, where the viewer count beside it has said that.
    /// </summary>
    [Fact]
    public void AnAbsentLatencyFigureCarriesTheReasonItIsAbsent()
    {
        var untimed = new HeaderStatsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), null, Serving(Rtmp("10.0.0.3:3"))),
        };

        Assert.Equal(Figure.NoValue, untimed.Figures[3].Value);
        Assert.EndsWith("watched over rtmp", untimed.Figures[3].Note);
        Assert.EndsWith("watched over rtmp", untimed.Figures[4].Note);

        // Only the two figures the reason is about: a note on the throughput would be about a leg it does not
        // concern.
        Assert.Null(untimed.Figures[0].Note);
        Assert.Null(untimed.Figures[1].Note);
        Assert.Null(untimed.Figures[5].Note);

        var timed = new HeaderStatsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), null, Serving(Srt("10.0.0.1:1", rttMs: 20, lossPercent: 0))),
        };

        Assert.Null(timed.Figures[3].Note);
        Assert.Null(timed.Figures[4].Note);

        var unwatched = new HeaderStatsViewModel { Snapshot = BroadcastSnapshot.Of(Live(), null, Serving()) };
        Assert.Null(unwatched.Figures[3].Note);
        Assert.Null(unwatched.Figures[4].Note);
    }

    /// <summary>Two snapshots make a curve; one is a reading and not a shape.</summary>
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
            RelaySamples = samples.Select((sample, second) => Reading(sample, second)).ToList(),
        };

        Assert.True(plots.HasLatency);
        Assert.Equal(3, plots.Rtt.Count);
        Assert.Equal(3, plots.Loss.Count);
        Assert.Equal("", plots.LatencyNotice);

        // Y grows downward, so the highest round trip has the smallest Y.
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
            RelaySamples = [Reading(Serving())],
        };
        Assert.False(unwatched.HasLatency);
        Assert.Equal("nobody is watching yet", unwatched.LatencyNotice);

        // Viewers, none of them on a timed leg.
        // The sentence names the legs they are on, which is the half a publisher can act on.
        var untimed = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), null, Serving(Rtmp("10.0.0.3:3"))),
            RelaySamples = [Reading(Serving(Rtmp("10.0.0.3:3")))],
        };
        Assert.False(untimed.HasLatency);
        Assert.Contains("srt", untimed.LatencyNotice);
        Assert.EndsWith("watched over rtmp", untimed.LatencyNotice);

        // Timed once: a measurement and no shape yet.
        // "Nobody is timed" would contradict the figure the header shows off the same snapshot.
        var once = Serving(Srt("10.0.0.1:1", rttMs: 20, lossPercent: 0));
        var starting = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), null, once),
            RelaySamples = [Reading(once)],
        };
        Assert.False(starting.HasLatency);
        Assert.Equal("waiting for the relay's next snapshot", starting.LatencyNotice);
        Assert.Equal(20, starting.Snapshot.RttMs);
    }

    /// <summary>
    /// The axis ends at the newest snapshot rather than at the newest one that timed somebody, so a reading
    /// out of the window leaves the plot instead of being drawn beside a header saying nobody is timed.
    /// </summary>
    [Fact]
    public void ACurveOfReadingsOlderThanTheWindowLeavesThePlot()
    {
        var timed = Serving(Srt("10.0.0.1:1", rttMs: 20, lossPercent: 0));
        var since = Serving(Rtmp("10.0.0.3:3"));

        var plots = new PlotsViewModel
        {
            Snapshot = BroadcastSnapshot.Of(Live(), null, since),
            RelaySamples =
            [
                Reading(timed, second: 1),
                Reading(timed, second: 3),
                Reading(since, second: 200),
            ],
        };

        Assert.False(plots.HasLatency);
        Assert.Empty(plots.Rtt);
        Assert.EndsWith("watched over rtmp", plots.LatencyNotice);
    }

    /// <summary>
    /// Nothing announces a viewer.
    /// The relay reports who is connected at each poll, so who arrived and who left exists only as the
    /// difference between two of those answers.
    /// </summary>
    [Fact]
    public void ArrivingAndLeavingAreTheDifferenceBetweenTwoRosters()
    {
        var first = Srt("10.0.0.1:1", rttMs: 20, lossPercent: 0);
        var second = Rtmp("10.0.0.2:2");

        var changes = Audience.Of(
        [
            Reading(Serving(first), second: 1),
            Reading(Serving(first, second), second: 2),
            Reading(Serving(second), second: 3),
        ],
        "desk");

        Assert.Equal(3, changes.Count);

        // The first roster's readers are arrivals, stamped with the relay's join time rather than the poll's.
        Assert.True(changes[0].Arrived);
        Assert.Equal("10.0.0.1:1", changes[0].Name);
        Assert.Equal("srt", changes[0].Via);
        Assert.Equal(DateTimeOffset.Parse("2026-08-09T22:01:12+00:00"), changes[0].At);

        Assert.True(changes[1].Arrived);
        Assert.Equal("10.0.0.2:2", changes[1].Name);

        // A departure has no stamp anywhere, so it is dated by the poll that first did not name the reader.
        Assert.False(changes[2].Arrived);
        Assert.Equal("10.0.0.1:1", changes[2].Name);
        Assert.Equal("srt", changes[2].Via);
        Assert.Equal(3, changes[2].At.Second);

        var line = LogLine.Of(changes[2]);
        Assert.Equal("INFO", line.Level);
        Assert.Equal("10.0.0.1:1 stopped watching over srt", line.Message);
        Assert.Equal("10.0.0.1:1 started watching over srt", LogLine.Of(changes[0]).Message);
    }

    /// <summary>
    /// A poll that named no path for this stream says nothing about who is watching.
    /// Read as an empty roster it would log a departure for every viewer each time the relay was unreachable,
    /// and an arrival for every one of them on the poll after.
    /// </summary>
    [Fact]
    public void APollThatSawNoPathIsNotEverybodyLeaving()
    {
        var watching = Srt("10.0.0.1:1", rttMs: 20, lossPercent: 0);

        var changes = Audience.Of(
        [
            Reading(Serving(watching), second: 1),
            Reading(new RelayStatus { Reachable = false, Error = "connection refused" }, second: 2),
            Reading(Serving(watching), second: 3),
        ],
        "desk");

        var arrival = Assert.Single(changes);
        Assert.True(arrival.Arrived);
    }

    /// <summary>
    /// A stream nothing is publishing has no path to read a roster off, so it produces no lines rather than
    /// the readers of a path that happens to carry the empty name.
    /// </summary>
    [Fact]
    public void AStreamWithNoNameHasNoAudience()
    {
        var changes = Audience.Of([Reading(Serving(Rtmp("10.0.0.2:2")))], "");

        Assert.Empty(changes);
    }
}
