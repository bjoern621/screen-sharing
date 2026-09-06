using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Insights.Model;

/// <summary>One viewer starting or stopping watching this stream.</summary>
/// <param name="Name">As <see cref="Readers.NameOf"/> gives it.</param>
/// <param name="Via">Leg it was watching over, in the transport vocabulary.</param>
/// <param name="Arrived">True for an arrival, false for a departure.</param>
/// <param name="At">Relay's join time on an arrival, the poll's own time on a departure.</param>
public sealed record AudienceChange(string Name, string Via, bool Arrived, DateTimeOffset At);

/// <summary>
/// Who arrived and who left, derived from the relay snapshots the session holds.
///
/// <b>The relay reports who is connected and never a disconnect.</b> The contract carries no arrival and no
/// departure event, so either fact exists only between two consecutive rosters.
/// Derived from the stored series on every pass rather than accumulated as viewers come and go: two passes
/// over one series produce the same lines, and a shell that reconnected and read again loses no change it was not
/// watching for (<c>docs/development-principles.md</c>, "Stateless").
///
/// <b>Two clocks meet here, each dating the fact it can.</b> An arrival carries the relay's join time, the moment
/// it happened.
/// A departure has no stamp, the relay stopping naming the reader and saying nothing about when, so it carries
/// the arrival time of the poll that first did not name it.
///
/// <b>A snapshot with no path for this stream contributes nothing, and no empty roster is read out of it.</b>
/// An unreachable relay, a poll older than the stream's path and a restarted relay all arrive that way, and
/// reading any of them as "everybody left" logs a departure per viewer on every relay hiccup.
///
/// Nothing before the oldest snapshot the session still holds is visible here.
/// The series is bounded and cleared when a stream stops, so these lines cover the run on screen, and the whole
/// history is the file behind "Open full log".
/// </summary>
public static class Audience
{
    /// <summary>
    /// Every arrival and departure in the given snapshots, oldest first.
    /// Readers of the first snapshot naming this stream's path count as arrivals, dated by the relay's join time,
    /// so a viewer that connected before this shell looked is dated by the connection, not by the poll.
    /// </summary>
    public static IReadOnlyList<AudienceChange> Of(IReadOnlyList<RelayReading> readings, string stream)
    {
        Assert.NotNull(readings, "an audience is read off the snapshots that were taken");
        Assert.NotNull(stream, "an audience watches one stream's path");

        var changes = new List<AudienceChange>();
        IReadOnlyList<RelayReader> before = [];
        var held = new HashSet<string>();
        var read = false;

        foreach (var reading in readings)
        {
            var path = InsightsSnapshot.PathOf(reading.Status, stream);
            if (path is null)
            {
                continue;
            }

            var now = path.ReaderRoster;
            var connected = Ids(now);

            foreach (var reader in now)
            {
                if (!held.Contains(reader.Id))
                {
                    // Relay's join time where it stated one, the poll's otherwise.
                    // Fallback reached only for a reader the relay described nowhere, the one whose address
                    // is missing from the table beside this.
                    var at = Readers.JoinedAt(reader) ?? reading.At;
                    changes.Add(new AudienceChange(Readers.NameOf(reader), Via(reader), Arrived: true, at));
                }
            }

            // Departures only against a roster that was read.
            // The first snapshot otherwise diffs against an empty roster in both directions, right for arrivals,
            // and right for departures only if an empty roster meant "nobody was watching" and not "nobody has
            // looked".
            if (read)
            {
                foreach (var reader in before)
                {
                    if (!connected.Contains(reader.Id))
                    {
                        changes.Add(new AudienceChange(Readers.NameOf(reader), Via(reader), Arrived: false, reading.At));
                    }
                }
            }

            before = now;
            held = connected;
            read = true;
        }

        return changes;
    }

    /// <summary>
    /// Relay handles on one snapshot's readers.
    /// The handle is the identity, the address is not: a reconnect is a new connection from the same address,
    /// a viewer that left and came back.
    /// </summary>
    private static HashSet<string> Ids(IReadOnlyList<RelayReader> roster)
    {
        var ids = new HashSet<string>(roster.Count);
        foreach (var reader in roster)
        {
            ids.Add(reader.Id);
        }

        return ids;
    }

    /// <summary>Leg, or the ellipsis where the relay named no transport for the reader.</summary>
    private static string Via(RelayReader reader) => reader.Transport.Length > 0 ? reader.Transport : Figure.NoValue;
}
