using System.Net;
using ScreenShare.App.Relay;
using Xunit;

namespace ScreenShare.App.Tests;

public sealed class RelayClientTests
{
    private const string OnePath = """
        {"items":[{"name":"bjoern","ready":true,"tracks":["H264","MPEG-4 Audio"],
        "bytesReceived":1000000,"readers":[{"type":"webrtcSession"},{"type":"srtConn"}]}]}
        """;

    private static string OnePathWithBytes(long bytes) => $$"""
        {"items":[{"name":"bjoern","ready":true,"tracks":["H264"],
        "bytesReceived":{{bytes}},"readers":[]}]}
        """;

    private static (RelayClient Client, StubHandler Handler, StubClock Clock) Build(
        Func<HttpRequestMessage, HttpResponseMessage> answer)
    {
        var handler = new StubHandler(answer);
        var clock = new StubClock();
        var http = new HttpClient(handler) { Timeout = TimeSpan.FromSeconds(3) };
        return (new RelayClient(http, clock), handler, clock);
    }

    [Fact]
    public async Task ThePollAsksTheRelayForItsPathList()
    {
        var (client, handler, _) = Build(_ => StubHandler.Json(OnePath));

        await client.FetchAsync("relay.example", 9997);

        Assert.Equal("http://relay.example:9997/v3/paths/list", handler.LastUri?.ToString());
    }

    [Fact]
    public async Task APathCarriesItsFormatReadersAndTracks()
    {
        var (client, _, _) = Build(_ => StubHandler.Json(OnePath));

        var status = await client.FetchAsync("127.0.0.1", 9997);

        var path = Assert.Single(status.Paths);
        Assert.True(status.Reachable);
        Assert.Equal("bjoern", path.Name);
        Assert.Equal("h264", path.Format);
        Assert.Equal("H264,MPEG-4 Audio", path.Tracks);
        Assert.Equal(2, path.Readers);
        Assert.True(path.Ready);
    }

    [Fact]
    public async Task TheFirstPollHasNoBitrateAndTheSecondDerivesItFromTheDelta()
    {
        var bytes = 1_000_000L;
        var (client, _, clock) = Build(_ => StubHandler.Json(OnePathWithBytes(bytes)));

        var first = await client.FetchAsync("127.0.0.1", 9997);
        Assert.Equal(0, Assert.Single(first.Paths).InMbps);

        // 250 kB in one second is 2 Mbit/s.
        bytes += 250_000;
        clock.Advance(TimeSpan.FromSeconds(1));
        var second = await client.FetchAsync("127.0.0.1", 9997);

        Assert.Equal(2.0, Assert.Single(second.Paths).InMbps, precision: 6);
    }

    [Fact]
    public async Task AVanishedPathStartsAFreshDelta()
    {
        var body = OnePathWithBytes(5_000_000);
        var (client, _, clock) = Build(_ => StubHandler.Json(body));

        await client.FetchAsync("127.0.0.1", 9997);

        // The publisher leaves, so the relay drops the path and its counter with it.
        body = """{"items":[]}""";
        clock.Advance(TimeSpan.FromSeconds(1));
        var empty = await client.FetchAsync("127.0.0.1", 9997);
        Assert.Empty(empty.Paths);

        // It comes back with a counter that restarted. Without forgetting the old sample
        // the delta would be negative, and a naive one would report a huge bitrate.
        body = OnePathWithBytes(1_000);
        clock.Advance(TimeSpan.FromSeconds(1));
        var back = await client.FetchAsync("127.0.0.1", 9997);

        Assert.Equal(0, Assert.Single(back.Paths).InMbps);
    }

    [Fact]
    public async Task AnUnreachableRelayIsReportedInsideTheStatus()
    {
        var (client, _, _) = Build(_ => throw new HttpRequestException("connection refused"));

        var status = await client.FetchAsync("127.0.0.1", 9997);

        Assert.Equal(RelayReach.Unreachable, status.Reach);
        Assert.Contains("connection refused", status.Error);
        Assert.Empty(status.Paths);
    }

    [Fact]
    public async Task ARefusedRequestIsReportedWithItsStatusCode()
    {
        var (client, _, _) = Build(_ => StubHandler.Status(HttpStatusCode.Unauthorized));

        var status = await client.FetchAsync("127.0.0.1", 9997);

        Assert.Equal(RelayReach.Unreachable, status.Reach);
        Assert.Contains("401", status.Error);
    }

    [Fact]
    public async Task AMalformedAnswerIsAnEnvironmentFailureAndNotAThrow()
    {
        var (client, _, _) = Build(_ => StubHandler.Json("not json at all"));

        var status = await client.FetchAsync("127.0.0.1", 9997);

        Assert.Equal(RelayReach.Unreachable, status.Reach);
        Assert.Contains("invalid API response", status.Error);
    }
}
