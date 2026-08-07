using System.Text.Json;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Relay;

/// <summary>
/// Talks to the MediaMTX HTTP API for stream discovery, the C# counterpart of
/// <c>desktop/internal/relay</c>.
///
/// The relay is the single source of truth for "who is live". Live per-path bitrates
/// are derived from bytesReceived deltas between two polls - the API itself only
/// exposes counters, which is why the client is stateful and why one instance has to
/// serve every poll of one relay.
///
/// Safe for concurrent use.
/// </summary>
public sealed class RelayClient
{
    private readonly HttpClient _http;
    private readonly TimeProvider _time;
    private readonly Lock _gate = new();
    private readonly Dictionary<string, ByteSample> _previous = new(StringComparer.Ordinal);

    private readonly record struct ByteSample(long Bytes, DateTimeOffset At);

    public RelayClient(HttpClient http, TimeProvider time)
    {
        Assert.NotNull(http, "a relay client needs an HTTP client to poll through");
        Assert.NotNull(time, "a bitrate delta needs a clock to divide by");

        _http = http;
        _time = time;
    }

    /// <summary>
    /// Queries the relay once and returns the snapshot. An unreachable relay is an
    /// environment condition, not an error - it is reported inside the status so the UI
    /// can display it. Only a cancellation the caller asked for propagates.
    /// </summary>
    public async Task<RelayStatus> FetchAsync(string host, int apiPort, CancellationToken cancellation = default)
    {
        Assert.That(!string.IsNullOrWhiteSpace(host), "a relay poll names a host", host);
        Assert.That(apiPort is > 0 and <= 65535, "a relay API port is a TCP port number", apiPort);

        var url = $"http://{host}:{apiPort}/v3/paths/list";

        ApiPathList? list;
        try
        {
            using var response = await _http.GetAsync(url, cancellation).ConfigureAwait(false);
            if (!response.IsSuccessStatusCode)
            {
                return RelayStatus.Failed($"relay answered {(int)response.StatusCode} {response.ReasonPhrase}");
            }

            var body = await response.Content.ReadAsStreamAsync(cancellation).ConfigureAwait(false);
            await using (body.ConfigureAwait(false))
            {
                list = await JsonSerializer
                    .DeserializeAsync(body, RelayJson.Default.ApiPathList, cancellation)
                    .ConfigureAwait(false);
            }
        }
        catch (OperationCanceledException) when (cancellation.IsCancellationRequested)
        {
            throw;
        }
        catch (OperationCanceledException)
        {
            // Not the caller's cancellation, so it is the HttpClient timeout firing.
            return RelayStatus.Failed($"no answer within {_http.Timeout.TotalSeconds:0.#}s");
        }
        catch (HttpRequestException failure)
        {
            return RelayStatus.Failed(failure.Message);
        }
        catch (JsonException failure)
        {
            return RelayStatus.Failed("invalid API response: " + failure.Message);
        }

        if (list is null)
        {
            return RelayStatus.Failed("invalid API response: empty body");
        }

        return Snapshot(list);
    }

    /// <summary>
    /// Turns one API response into a status, folding in the byte deltas held since the
    /// previous poll. Separated from the request so the delta arithmetic is reachable
    /// without a socket.
    /// </summary>
    private RelayStatus Snapshot(ApiPathList list)
    {
        var now = _time.GetUtcNow();
        var paths = new List<RelayPath>(list.Items.Count);
        var seen = new HashSet<string>(StringComparer.Ordinal);

        lock (_gate)
        {
            foreach (var item in list.Items)
            {
                seen.Add(item.Name);

                var inMbps = 0.0;
                if (_previous.TryGetValue(item.Name, out var previous))
                {
                    var seconds = (now - previous.At).TotalSeconds;
                    if (seconds > 0 && item.BytesReceived >= previous.Bytes)
                    {
                        inMbps = (item.BytesReceived - previous.Bytes) * 8 / seconds / 1e6;
                    }
                }

                _previous[item.Name] = new ByteSample(item.BytesReceived, now);

                paths.Add(new RelayPath
                {
                    Name = item.Name,
                    Ready = item.Ready,
                    Tracks = string.Join(",", item.Tracks),
                    Format = TrackFormats.Of(item.Tracks),
                    Readers = item.Readers.Count,
                    InMbps = inMbps,
                });
            }

            // Forget paths that vanished, so a re-appearing stream starts a fresh delta
            // instead of dividing a reset counter by the time it was away.
            foreach (var name in _previous.Keys.Where(name => !seen.Contains(name)).ToList())
            {
                _previous.Remove(name);
            }

            Assert.That(_previous.Count == seen.Count, "one byte sample per path the relay reported",
                _previous.Count, seen.Count);
        }

        return RelayStatus.Live(paths);
    }
}
