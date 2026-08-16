using System.Globalization;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// Two facts about a relay reader more than one card states: who it is, and when the relay accepted it.
/// One place: the viewer table and the session log describe the same connections, and two spellings of "who is
/// this" let a row and a line disagree about one viewer while reading one roster.
/// </summary>
public static class Readers
{
    /// <summary>
    /// Who a reader is: the address the relay saw, its connection handle where the relay gave no address, and
    /// <see cref="Figure.NoValue"/> where it stated neither.
    /// Falls back and then gives up rather than asserting.
    /// These are the relay's own words, so a reader it named nothing is an Umgebungsfehler this screen survives:
    /// an unnameable viewer still reads as somebody connected.
    /// </summary>
    public static string NameOf(RelayReader reader)
    {
        Assert.NotNull(reader, "a name describes a reader the relay reported");

        var address = reader.HasRemoteAddr && reader.RemoteAddr.Length > 0 ? reader.RemoteAddr : reader.Id;
        return address.Length > 0 ? address : Figure.NoValue;
    }

    /// <summary>
    /// When the relay accepted this reader, null where it said nothing parseable.
    /// The relay's clock, only the relay being able to date its own connections: this converts an instant and
    /// never composes one.
    /// </summary>
    public static DateTimeOffset? JoinedAt(RelayReader reader)
    {
        Assert.NotNull(reader, "a join time describes a reader the relay reported");

        if (!reader.HasJoined || !DateTimeOffset.TryParse(
                reader.Joined, CultureInfo.InvariantCulture, DateTimeStyles.RoundtripKind, out var joined))
        {
            return null;
        }

        return joined;
    }
}
