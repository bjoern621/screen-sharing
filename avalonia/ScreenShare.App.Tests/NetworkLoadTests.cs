using ScreenShare.Api.V1;
using ScreenShare.App.Features.Shell.StatusBar.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The band prints this machine's own traffic and nothing else.
///
/// Defect locked out: a relay's whole ingest read as a figure about this computer.
/// Another member's publisher costs this machine nothing, and a synthetic set running here costs it everything,
/// so a figure summed over the relay's paths says the opposite of what it is read as.
/// </summary>
public sealed class NetworkLoadTests
{
    private static RelayPath Path(string name, double mbps) => new()
    {
        Name = name,
        Ready = true,
        InMbps = mbps,
    };

    private static RelayStatus Relay(params RelayPath[] paths)
    {
        var status = new RelayStatus { Reachable = true };
        status.Paths.AddRange(paths);
        return status;
    }

    private static TestStreamSlot Slot(int slot, string name, bool running = true) => new()
    {
        Slot = slot,
        Name = name,
        Running = running,
    };

    private static ReceiveStreamStats Decode(string name, double? mbps) 
    {
        var stats = new ReceiveStreamStats { Stream = new StreamRef { StreamName = name, Transport = "srt" } };
        if (mbps is { } rate)
        {
            stats.VideoMbps = rate;
        }

        return stats;
    }

    /// <summary>
    /// The reported defect: a synthetic set publishing from this machine, watched by nobody here.
    /// Every byte of it leaves this computer, so it is the sending figure and there is nothing to receive.
    /// </summary>
    [Fact]
    public void ASyntheticSetThisMachineRunsIsSentRatherThanReceived()
    {
        var load = NetworkLoad.Of(
            Relay(Path("bjoern/test-1", 1.7), Path("bjoern/test-2-hdr", 1.7), Path("bjoern/test-3-audio", 1.7)),
            publishing: "",
            [Slot(0, "bjoern/test-1"), Slot(1, "bjoern/test-2-hdr"), Slot(2, "bjoern/test-3-audio")],
            []);

        Assert.Equal(["sending 5.1 Mb/s"], load);
    }

    /// <summary>
    /// A path another member publishes crosses that member's connection and the relay.
    /// </summary>
    [Fact]
    public void AnotherMembersStreamIsNotThisMachinesTraffic()
    {
        var load = NetworkLoad.Of(Relay(Path("someone/desk", 8.0)), publishing: "", [], []);

        Assert.Empty(load);
    }

    /// <summary>
    /// Watching that stream is what puts it on this machine's connection, and only the decode counts it.
    /// </summary>
    [Fact]
    public void AWatchedStreamCountsOnceAsReceived()
    {
        var load = NetworkLoad.Of(
            Relay(Path("someone/desk", 8.0)),
            publishing: "",
            [],
            [Decode("someone/desk", 8.0)]);

        Assert.Equal(["receiving 8.0 Mb/s"], load);
    }

    /// <summary>
    /// The stream this machine publishes is its upload, and the band states both directions at once.
    /// </summary>
    [Fact]
    public void ThePublishedStreamIsSentBesideWhatIsWatched()
    {
        var load = NetworkLoad.Of(
            Relay(Path("bjoern/desk", 6.0), Path("someone/desk", 8.0)),
            publishing: "bjoern/desk",
            [],
            [Decode("someone/desk", 2.5)]);

        Assert.Equal(["receiving 2.5 Mb/s", "sending 6.0 Mb/s"], load);
    }

    /// <summary>
    /// A slot waiting out its backoff publishes nothing, whatever the relay still reports for the path it holds.
    /// </summary>
    [Fact]
    public void ASlotThatIsNotRunningSendsNothing()
    {
        var load = NetworkLoad.Of(
            Relay(Path("bjoern/test-1", 1.7)),
            publishing: "",
            [Slot(0, "bjoern/test-1", running: false)],
            []);

        Assert.Empty(load);
    }

    /// <summary>
    /// An open decode nothing has measured carries no rate,
    /// and a zero would read as a stream delivering nothing.
    /// </summary>
    [Fact]
    public void AnUnmeasuredDecodeLeavesTheReceivingFigureOut()
    {
        var load = NetworkLoad.Of(Relay(Path("someone/desk", 8.0)), publishing: "", [], [Decode("someone/desk", null)]);

        Assert.Empty(load);
    }

    /// <summary>
    /// An idle machine states no figures, and the band draws its height with nothing in it.
    /// </summary>
    [Fact]
    public void AnIdleMachineStatesNothing()
    {
        Assert.Empty(NetworkLoad.Of(null, publishing: "", [], []));
        Assert.Empty(NetworkLoad.Of(Relay(), publishing: "", [], []));
    }
}
