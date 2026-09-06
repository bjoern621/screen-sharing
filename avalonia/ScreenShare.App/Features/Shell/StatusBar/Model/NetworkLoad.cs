using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Shell.StatusBar.Model;

/// <summary>
/// What this computer's connection carries, in both directions.
///
/// A relay path this machine does not publish is another member's upload and crosses this connection only once
/// something here watches it,
/// so the sent figure is summed over the paths this machine claims and the received one over the open decodes.
/// Neither counts the other's bytes: a stream published here and tiled here is one upload and one download.
///
/// The two are measured at different ends, there being no one meter across both.
/// Sending is the relay's own ingest reading, the synthetic set running as children that report no rate of their
/// own,
/// and receiving is what each decode pipeline measured.
/// </summary>
public static class NetworkLoad
{
    /// <summary>
    /// The figures the band prints, in the order it prints them, and empty for a machine carrying nothing.
    /// </summary>
    /// <param name="relay">Last snapshot, null while none has arrived and unreachable after a failed poll.</param>
    /// <param name="publishing">Relay path of the running publish, empty while nothing publishes.</param>
    /// <param name="slots">Synthetic set, whose running slots publish from here as a capture does.</param>
    /// <param name="decodes">Last sample per open decode.</param>
    public static IReadOnlyList<string> Of(
        RelayStatus? relay,
        string publishing,
        IReadOnlyList<TestStreamSlot> slots,
        IReadOnlyList<ReceiveStreamStats> decodes)
    {
        Assert.NotNull(publishing, "a load names the path this machine publishes, or none");
        Assert.NotNull(slots, "a load is told which synthetic slots are up");
        Assert.NotNull(decodes, "a load is told what every open decode last measured");

        var figures = new List<string>(2);

        var received = 0.0;
        var measured = false;
        foreach (var decode in decodes)
        {
            if (decode.HasVideoMbps)
            {
                received += decode.VideoMbps;
                measured = true;
            }
        }

        if (measured)
        {
            figures.Add($"receiving {received:0.0} Mb/s");
        }

        var sent = 0.0;
        var sends = false;
        if (relay is { Reachable: true })
        {
            foreach (var path in relay.Paths)
            {
                if (path.Ready && Ours(path.Name, publishing, slots))
                {
                    // Zero until the poll after the one that found the path, a rate being a byte delta between
                    // two of them.
                    sent += path.InMbps;
                    sends = true;
                }
            }
        }

        if (sends)
        {
            figures.Add($"sending {sent:0.0} Mb/s");
        }

        Assert.That(figures.Count <= 2, "a load states each direction at most once", figures.Count);
        return figures;
    }

    /// <summary>
    /// Whether this machine is the one uploading the path.
    /// Matched on the name because that is what both sides carry
    /// (<c>api/proto/screenshare/v1/session.proto</c>, PublishState.Live.stream_name):
    /// a publish states the string a snapshot's row is found by,
    /// and a slot names the path it publishes to.
    /// </summary>
    private static bool Ours(string path, string publishing, IReadOnlyList<TestStreamSlot> slots)
    {
        if (publishing.Length > 0 && path == publishing)
        {
            return true;
        }

        foreach (var slot in slots)
        {
            if (slot.Running && slot.Name == path)
            {
                return true;
            }
        }

        return false;
    }
}
