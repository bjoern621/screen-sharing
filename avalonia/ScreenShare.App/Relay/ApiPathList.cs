using System.Text.Json;
using System.Text.Json.Serialization;

namespace ScreenShare.App.Relay;

/// <summary>Mirrors the subset of <c>GET /v3/paths/list</c> this app consumes.</summary>
internal sealed class ApiPathList
{
    [JsonPropertyName("items")]
    public List<ApiPath> Items { get; set; } = [];
}

internal sealed class ApiPath
{
    [JsonPropertyName("name")]
    public string Name { get; set; } = "";

    [JsonPropertyName("ready")]
    public bool Ready { get; set; }

    [JsonPropertyName("tracks")]
    public List<string> Tracks { get; set; } = [];

    [JsonPropertyName("bytesReceived")]
    public long BytesReceived { get; set; }

    /// <summary>
    /// Only the count is read, so the elements stay unparsed. Reader objects carry
    /// per-protocol fields this app has no use for and would otherwise have to track.
    /// </summary>
    [JsonPropertyName("readers")]
    public List<JsonElement> Readers { get; set; } = [];
}

/// <summary>
/// Source-generated serialisation for the relay API. Reflection-free, so the reader
/// keeps working under trimming or AOT if this project ever publishes that way.
/// </summary>
[JsonSourceGenerationOptions(PropertyNameCaseInsensitive = true)]
[JsonSerializable(typeof(ApiPathList))]
internal sealed partial class RelayJson : JsonSerializerContext;
