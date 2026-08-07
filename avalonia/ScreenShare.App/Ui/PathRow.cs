using ScreenShare.App.Relay;

namespace ScreenShare.App.Ui;

/// <summary>
/// One relay path as the list renders it. A record, so two passes over an unchanged
/// snapshot compare equal and the render function can leave the list alone.
///
/// Figures keep the names they have on every other surface, spell their words out, and
/// print an ellipsis where there is no value yet (docs/design-language.md, "Wording").
/// </summary>
public sealed record PathRow(string Name, string Format, string Tracks, string Readers, string Bitrate, bool Ready)
{
    private const string NoValue = "…";

    public static PathRow From(RelayPath path) => new(
        Name: path.Name,
        Format: path.Format.Length > 0 ? path.Format : NoValue,
        Tracks: path.Tracks.Length > 0 ? path.Tracks : NoValue,
        Readers: $"{path.Readers} watching",
        Bitrate: path.InMbps > 0 ? $"{path.InMbps:0.0} Mbps" : NoValue,
        Ready: path.Ready);
}
