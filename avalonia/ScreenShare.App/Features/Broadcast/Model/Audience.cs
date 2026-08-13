using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// One viewer starting or stopping watching this stream.
/// </summary>
/// <param name="Name">Who the reader is, as <see cref="Readers.NameOf"/> names it.</param>
/// <param name="Via">The leg it was watching over, in the transport vocabulary.</param>
/// <param name="Arrived">Whether this is an arrival.
/// False is a departure.</param> <param name="At">When it happened, on the clock named in
/// <see cref="Audience"/>.</param>
public sealed record AudienceChange(string Name, string Via, bool Arrived, DateTimeOffset At);

/// <summary>
/// Who arrived and who left, derived from the relay snapshots the session holds.
///
/// <b>The relay reports who is connected and never reports that somebody disconnected.</b> There is no
/// arrival event and no departure event on the contract, so the only place either fact exists is between two
/// consecutive rosters.
/// This reads it out of the series rather than accumulating it as viewers come and go: the session stores
/// whole snapshots in order and this is a function of them, so a pass that runs twice produces the same lines
/// and a shell that reconnected and read again does not lose the ones it was not watching for
/// (<c>docs/development-principles.md</c>, "Stateless").
///
/// <b>Two clocks meet here, and each is used for the fact it can date.</b> An arrival carries the relay's own
/// join time, which is the moment it happened.
/// A departure has no such stamp - the relay stops naming the reader and says nothing about when it went - so
/// it carries the arrival time of the poll that first did not name it, which is the honest reading of "this
/// is when this shell learned it".
/// The two are seconds apart on one machine and the line says which moment it is describing by being a log
/// line at all.
///
/// <b>A snapshot with no path for this stream contributes nothing rather than an empty roster.</b> An
/// unreachable relay, a poll taken before the stream had a path, and a relay that restarted all come out that
/// way, and reading any of them as "everybody left" would put a departure in the log for every viewer each
/// time the relay hiccupped.
///
/// What this cannot see is what happened before the oldest snapshot the session still holds.
/// The series is bounded and it is cleared when a stream stops, so these lines describe the run on screen;
/// the whole history is the file behind "Open full log".
/// </summary>
public static class Audience
{
    /// <summary>
    /// Every arrival and departure the given snapshots contain, oldest first.
    /// The readers of the first snapshot that names this stream's path are arrivals: they are watching, and
    /// the join time they carry is the relay's rather than this shell's, so a viewer that connected before
    /// the shell was looking is dated correctly instead of being dated by the poll that found it.
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
            var path = BroadcastSnapshot.PathOf(reading.Status, stream);
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
                    // The relay's join time where it stated one, and the poll's otherwise.
                    // The fallback is only reached for a reader the relay described nowhere, which is the
                    // same reader whose address is missing from the table beside this.
                    var at = Readers.JoinedAt(reader) ?? reading.At;
                    changes.Add(new AudienceChange(Readers.NameOf(reader), Via(reader), Arrived: true, at));
                }
            }

            // Only against a roster that was actually read.
            // Without this the first snapshot would diff against an empty roster in both directions, which is
            // right for arrivals - everybody in it is new to this reading - and would be right for departures
            // only if an empty roster meant "nobody was watching" rather than "nobody has looked".
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
    /// The relay's handles on one snapshot's readers.
    /// The handle is the identity and the address is not: a viewer that reconnects arrives on a new
    /// connection from the same address, which is a viewer that left and came back.
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

    /// <summary>The leg, or the ellipsis where the relay named the reader nothing this app has a transport for.</summary>
    private static string Via(RelayReader reader) => reader.Transport.Length > 0 ? reader.Transport : Figure.NoValue;
}
