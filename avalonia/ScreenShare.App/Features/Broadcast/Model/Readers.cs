using System.Globalization;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// The two facts about a relay reader that more than one card on this screen states: who it is, and when the
/// relay accepted it.
///
/// They live in one place because the viewer table and the session log describe the same connections, and two
/// spellings of "who is this" would let a row and a line disagree about one viewer while both were reading
/// the same roster.
/// </summary>
public static class Readers
{
    /// <summary>
    /// Who a reader is: the address the relay saw, its handle on the connection where the relay described it
    /// nowhere, and <see cref="Figure.NoValue"/> where it stated neither.
    ///
    /// It falls back and then gives up rather than asserting.
    /// These are the relay's own words, and a relay that named a reader nothing is an environment condition
    /// this screen has to survive - an unnameable viewer still reads as somebody who is connected.
    /// </summary>
    public static string NameOf(RelayReader reader)
    {
        Assert.NotNull(reader, "a name describes a reader the relay reported");

        var address = reader.HasRemoteAddr && reader.RemoteAddr.Length > 0 ? reader.RemoteAddr : reader.Id;
        return address.Length > 0 ? address : Figure.NoValue;
    }

    /// <summary>
    /// When the relay accepted this reader, and null where it said nothing this shell could parse.
    /// It is the relay's clock: the relay is the only side that can date its own connections, so this
    /// converts the instant and never composes one.
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
