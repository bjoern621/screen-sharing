namespace ScreenShare.App.Relay;

/// <summary>
/// Maps the codec names a relay reports on a path to the bitstream formats the codec
/// table keys on. The relay names a track after the coding format, never after the
/// encoder that produced it, which is why one entry serves every encoder of a format.
///
/// Both spellings of the two H.26x formats appear, since a relay may report either the
/// ITU name or the MPEG one depending on how the stream was ingested.
///
/// A table rather than a switch, so every consumer reads the same facts
/// (docs/domain-model.md, docs/development-principles.md "Components").
/// </summary>
public static class TrackFormats
{
    private static readonly Dictionary<string, string> ByTrackName = new(StringComparer.OrdinalIgnoreCase)
    {
        ["H264"] = "h264",
        ["AVC"] = "h264",
        ["H265"] = "hevc",
        ["HEVC"] = "hevc",
        ["VP8"] = "vp8",
        ["VP9"] = "vp9",
        ["AV1"] = "av1",
    };

    /// <summary>
    /// The bitstream format of the video track among the ones a relay path reports, and
    /// an empty string when none of them names a format this app knows. A path carries at
    /// most one video track here, so the first match is the answer.
    /// </summary>
    public static string Of(IReadOnlyList<string> tracks)
    {
        foreach (var track in tracks)
        {
            if (ByTrackName.TryGetValue(track, out var format))
            {
                return format;
            }
        }

        return "";
    }
}
