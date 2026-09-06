using System.Collections.Specialized;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Insights.HeaderStats.ViewModel;
using ScreenShare.App.Features.Insights.Model;
using ScreenShare.App.Features.Insights.Plots.ViewModel;
using ScreenShare.App.Features.Insights.ViewerTable.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Row per reader the relay named, cell per figure it stated about that reader.
/// Unstated figure reads absent, never zero, so a viewer nobody timed never reads as a viewer with a perfect link.
/// </summary>
public sealed class ViewerRosterTests
{
    /// <summary>SRT reader: only leg the relay times and states a loss rate on.</summary>
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
    /// One snapshot as the session holds it: relay's answer, and when the shell took it.
    /// Stamp is the only clock a departure has.
    /// </summary>
    private static RelayReading Reading(RelayStatus status, int second = 0)
        => new(status, new DateTimeOffset(2026, 8, 9, 22, 2, 0, TimeSpan.Zero).AddSeconds(second));

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
        Live = new PublishState.Types.Live { Publish = new PublishSettings(), StreamName = "desk" },
    };

    private static ViewerTableViewModel Table(RelayStatus? relay)
    {
        var path = InsightsSnapshot.PathOf(relay, "desk");
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

        // Measured zero is a measurement, and prints as one.
        Assert.Equal("0", row.Dropped);
        Assert.Equal("rtmp", row.Via);
        Assert.False(row.IsStruggling);
    }

    [Fact]
    public void AReaderTheRelayDescribedNowhereIsStillNamed()
    {
        // Named on the path, absent from every per-protocol list: a relay with that listener off.
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
    /// Relay naming a reader nothing is an Umgebungsfehler, so the row renders unnameable rather than asserting.
    /// Somebody is connected either way.
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
    /// Relay is polled while the pointer rests on the card,
    /// so a clear-and-fill per poll would reset the scroll position of a table nothing happened to.
    /// </summary>
    [Fact]
    public void AnUnchangedRosterRenderedAgainTouchesNothing()
    {
        var relay = Serving(Srt("10.0.0.1:1", 20, 0.5), Rtmp("10.0.0.2:2"));
        var table = Table(relay);

        var changes = 0;
        ((INotifyCollectionChanged)table.Rows).CollectionChanged += (_, _) => changes++;

        // Same roster again, through the path the render pass reads it by.
        table.Reported = InsightsSnapshot.PathOf(relay, "desk")!.ReaderRoster.Select(ViewerRow.Of).ToList();
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

        // Card is drawn with nothing publishing too, a third absence and not an empty roster.
        var idle = new ViewerTableViewModel();
        Assert.False(idle.HasRows);
        Assert.Contains("Nothing is publishing", idle.Notice);
    }

    /// <summary>
    /// Header's round trip and loss are the worst viewer's own.
    /// A mean is a figure nobody is experiencing, and averages a single struggling viewer away.
    /// </summary>
    [Fact]
    public void TheHeaderPromotesTheWorstViewersFigures()
    {
        var relay = Serving(
            Srt("10.0.0.1:1", rttMs: 12, lossPercent: 0.1),
            Srt("10.0.0.2:2", rttMs: 240, lossPercent: 4.5),
            Rtmp("10.0.0.3:3"));

        var reading = InsightsSnapshot.Of(Live(), null, relay);

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
    /// Viewers on untimed legs are not a stream nobody is watching,
    /// so the header tells them apart with a viewer count beside two absences.
    /// </summary>
    [Fact]
    public void ViewersOnAnUntimedLegLeaveTheLatencyFiguresAbsent()
    {
        var reading = InsightsSnapshot.Of(Live(), null, Serving(Rtmp("10.0.0.3:3"), Rtmp("10.0.0.4:4")));

        Assert.Equal(2, reading.Viewers);
        Assert.Null(reading.RttMs);
        Assert.Null(reading.LossPercent);
    }

    /// <summary>
    /// Distinct and in roster order, the value being read as prose.
    /// A reader with no protocol named takes no part, so the sentence has no gap in it.
    /// </summary>
    [Fact]
    public void TheLegsAreTheDistinctTransportsOnTheRoster()
    {
        Assert.Equal("", InsightsSnapshot.Of(Live(), null, Serving()).Legs);

        var oneLeg = InsightsSnapshot.Of(Live(), null, Serving(Rtmp("10.0.0.3:3"), Rtmp("10.0.0.4:4")));
        Assert.Equal("RTMP", oneLeg.Legs);

        var two = InsightsSnapshot.Of(Live(), null, Serving(Srt("10.0.0.1:1", 20, 0), Rtmp("10.0.0.3:3")));
        Assert.Equal("SRT, RTMP", two.Legs);

        var unnamed = new RelayReader { Type = "somethingNew", Id = "id-1" };
        Assert.Equal("", InsightsSnapshot.Of(Live(), null, Serving(unnamed)).Legs);
    }

    /// <summary>
    /// Unmeasured promoted figure carries why, the reason being one a publisher can act on:
    /// viewers, none of them on a leg the relay times.
    /// A measured figure carries no note, and neither does a stream nobody is watching,
    /// the viewer count beside it having said so.
    /// </summary>
    [Fact]
    public void AnAbsentLatencyFigureCarriesTheReasonItIsAbsent()
    {
        var untimed = new HeaderStatsViewModel
        {
            Snapshot = InsightsSnapshot.Of(Live(), null, Serving(Rtmp("10.0.0.3:3"))),
        };

        Assert.Equal(Figure.NoValue, untimed.Figures[3].Value);
        Assert.EndsWith("watched over RTMP", untimed.Figures[3].Note);
        Assert.EndsWith("watched over RTMP", untimed.Figures[4].Note);

        // Only the two figures the reason is about; a note on throughput would name a leg it does not concern.
        Assert.Null(untimed.Figures[0].Note);
        Assert.Null(untimed.Figures[1].Note);
        Assert.Null(untimed.Figures[5].Note);

        var timed = new HeaderStatsViewModel
        {
            Snapshot = InsightsSnapshot.Of(Live(), null, Serving(Srt("10.0.0.1:1", rttMs: 20, lossPercent: 0))),
        };

        Assert.Null(timed.Figures[3].Note);
        Assert.Null(timed.Figures[4].Note);

        var unwatched = new HeaderStatsViewModel { Snapshot = InsightsSnapshot.Of(Live(), null, Serving()) };
        Assert.Null(unwatched.Figures[3].Note);
        Assert.Null(unwatched.Figures[4].Note);
    }

    /// <summary>Two snapshots make a curve, one alone is a reading and not a shape.</summary>
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
            Snapshot = InsightsSnapshot.Of(Live(), null, samples[^1]),
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
        var idle = new PlotsViewModel { Snapshot = InsightsSnapshot.Of(null, null, null) };
        Assert.False(idle.HasLatency);
        Assert.Equal("nothing is publishing", idle.LatencyNotice);

        var unwatched = new PlotsViewModel
        {
            Snapshot = InsightsSnapshot.Of(Live(), null, Serving()),
            RelaySamples = [Reading(Serving())],
        };
        Assert.False(unwatched.HasLatency);
        Assert.Equal("nobody is watching yet", unwatched.LatencyNotice);

        // Viewers, none of them on a timed leg.
        // Notice names the legs they are on, the half a publisher can act on.
        var untimed = new PlotsViewModel
        {
            Snapshot = InsightsSnapshot.Of(Live(), null, Serving(Rtmp("10.0.0.3:3"))),
            RelaySamples = [Reading(Serving(Rtmp("10.0.0.3:3")))],
        };
        Assert.False(untimed.HasLatency);
        Assert.Contains("SRT", untimed.LatencyNotice);
        Assert.EndsWith("watched over RTMP", untimed.LatencyNotice);

        // Timed once: a measurement, no shape.
        // "Nobody is timed" would contradict the figure the header shows off the same snapshot.
        var once = Serving(Srt("10.0.0.1:1", rttMs: 20, lossPercent: 0));
        var starting = new PlotsViewModel
        {
            Snapshot = InsightsSnapshot.Of(Live(), null, once),
            RelaySamples = [Reading(once)],
        };
        Assert.False(starting.HasLatency);
        Assert.Equal("waiting for the relay's next snapshot", starting.LatencyNotice);
        Assert.Equal(20, starting.Snapshot.RttMs);
    }

    /// <summary>
    /// Axis ends at the newest snapshot rather than the newest that timed somebody,
    /// so a reading out of the window leaves the plot instead of standing beside a header saying nobody is timed.
    /// </summary>
    [Fact]
    public void ACurveOfReadingsOlderThanTheWindowLeavesThePlot()
    {
        var timed = Serving(Srt("10.0.0.1:1", rttMs: 20, lossPercent: 0));
        var since = Serving(Rtmp("10.0.0.3:3"));

        var plots = new PlotsViewModel
        {
            Snapshot = InsightsSnapshot.Of(Live(), null, since),
            RelaySamples =
            [
                Reading(timed, second: 1),
                Reading(timed, second: 3),
                Reading(since, second: 200),
            ],
        };

        Assert.False(plots.HasLatency);
        Assert.Empty(plots.Rtt);
        Assert.EndsWith("watched over RTMP", plots.LatencyNotice);
    }

    /// <summary>
    /// Nothing announces a viewer.
    /// Relay reports who is connected at each poll,
    /// so arrivals and departures exist only as the difference between two answers.
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

        // First roster's readers are arrivals, stamped with the relay's join time rather than the poll's.
        Assert.True(changes[0].Arrived);
        Assert.Equal("10.0.0.1:1", changes[0].Name);
        Assert.Equal("srt", changes[0].Via);
        Assert.Equal(DateTimeOffset.Parse("2026-08-09T22:01:12+00:00"), changes[0].At);

        Assert.True(changes[1].Arrived);
        Assert.Equal("10.0.0.2:2", changes[1].Name);

        // Departure has no stamp anywhere, so it is dated by the first poll not naming the reader.
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
    /// A poll naming no path for this stream says nothing about who is watching.
    /// Read as an empty roster it logs a departure per viewer whenever the relay is unreachable,
    /// and an arrival per viewer on the poll after.
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
    /// A stream nothing is publishing has no path to read a roster off,
    /// so it produces no lines rather than the readers of a path carrying the empty name.
    /// </summary>
    [Fact]
    public void AStreamWithNoNameHasNoAudience()
    {
        var changes = Audience.Of([Reading(Serving(Rtmp("10.0.0.2:2")))], "");

        Assert.Empty(changes);
    }

    /// <summary>
    /// A member reads the group service's index, which counts the readers of a path and leaves their names
    /// at the service (<c>docs/ipc-api.md</c>).
    /// The count then stands with no rows behind it, an unavailable roster wearing the look of an empty one,
    /// and this card is where that difference is stated.
    /// </summary>
    [Fact]
    public void ACountWithNoRosterBehindItSaysTheViewersAreUnnamed()
    {
        var path = new RelayPath { Name = "desk", Ready = true, Readers = 2 };
        var relay = new RelayStatus { Reachable = true };
        relay.Paths.Add(path);

        var table = Table(relay);

        Assert.False(table.HasRows);
        Assert.Equal(Cards.ViewersUnnamed, table.Notice);
    }
}
