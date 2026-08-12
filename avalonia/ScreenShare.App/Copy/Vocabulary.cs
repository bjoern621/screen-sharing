using System.Globalization;
using ScreenShare.Api.V1;

namespace ScreenShare.App.Copy;

/// <summary>
/// How this shell names one option of one control, and what it says about it.
///
/// The backend sends an entry as a bare value - <c>hevc_nvenc</c>, <c>2</c>,
/// <c>1280x720</c> - and which control it belongs to. Naming it needs both: <c>auto</c>
/// means one thing under the frame path and another under the download route, and no
/// table keyed on the value alone can tell them apart. So the field key is the switch and
/// the value is the lookup, which is also why a control added to the contract shows up
/// here as a row rather than as a change anywhere else.
///
/// Three entries need more than their own value to be named, and all three read the
/// catalog: a codec is named by the format and the family its row carries, a capture
/// backend by the engine that reads it, and a monitor by the size and refresh rate of the
/// output at that index. The catalog is optional and absent until the first read lands,
/// and every method answers without it - a codec falls back to its encoder name and a
/// screen to its index, which are exactly what the backend called them.
///
/// Two of those three are named against the whole table rather than off their own row,
/// because a name that repeats is a name that does not identify: the same screen is read
/// by both engines and the same format is produced by several encoders in one family, so
/// what separates the entries is what the name has to carry.
/// </summary>
public sealed class Vocabulary
{
    /// <summary>The one used before the catalog has arrived. Everything answers; some answers are shorter.</summary>
    public static readonly Vocabulary Empty = new(null);

    private readonly Catalog? _catalog;

    public Vocabulary(Catalog? catalog) => _catalog = catalog;

    /// <summary>What one entry of one control is called, in the width a dropdown row has.</summary>
    public string Name(string fieldKey, string value) => fieldKey switch
    {
        "publish.capture" => Capture(value),
        "publish.monitor" => Screen(value),
        "publish.output_resolution" => Resolution(value),
        "publish.fps" => Rate(value),
        "publish.capture_memory" => Words.Memory(value),
        "publish.drm_map" => Words.DrmMap(value),
        "publish.codec" => Codec(value),
        "publish.chroma" => Chroma(value),
        "publish.color_range" => Words.ColorRange(value),
        "publish.effort" => Words.Effort(value),
        "publish.tune" => Words.Tune(value),
        "publish.mode" => Words.Mode(value),
        "publish.audio" => Words.AudioSource(value),
        "publish.audio_codec" => Words.AudioCodec(value),
        "publish.publish_transport" or "viewer.player_watch_transport" or "viewer.tile_watch_transport"
            => Words.Transport(value),
        "publish.rtsp_publish_protocol" or "viewer.rtsp_watch_protocol" => Words.RtspProtocol(value),
        "viewer.render_chain" => Words.RenderChain(value),
        _ => value,
    };

    /// <summary>
    /// The paragraph behind one entry, and nothing where the name says it all. It is what
    /// a radio card shows under its title and what a dropdown shows where there is room.
    /// </summary>
    public string Describe(string fieldKey, string value) => fieldKey switch
    {
        "publish.capture" => Descriptions.Capture(value),
        "publish.output_resolution" => Scaling(value),
        "publish.capture_memory" => Descriptions.Memory(value),
        "publish.drm_map" => Descriptions.DrmMap(value),
        "publish.codec" => DescribeCodec(value),
        "publish.chroma" => Descriptions.Chroma(value),
        "publish.color_range" => Descriptions.ColorRange(value),
        "publish.effort" => Descriptions.Effort(value),
        "publish.tune" => Descriptions.Tune(value),
        "publish.mode" => Descriptions.Mode(value),
        "publish.audio" => Descriptions.AudioSource(value),
        "publish.audio_codec" => Descriptions.AudioCodec(value),
        "publish.publish_transport" or "viewer.player_watch_transport" or "viewer.tile_watch_transport"
            => Descriptions.Transport(value),
        "publish.rtsp_publish_protocol" or "viewer.rtsp_watch_protocol" => Descriptions.RtspProtocol(value),
        "viewer.render_chain" => Descriptions.RenderChain(value),
        _ => "",
    };

    /// <summary>
    /// What one group settled on, in the few words a step chip repeats.
    ///
    /// It is composed here and not received, because it is a shorthand: a separator, an
    /// abbreviation and a length, all decided by the strip it sits in. The values behind it
    /// are the draft's own, so it cannot say anything the form does not.
    ///
    /// A group with nothing worth a line answers with nothing rather than with a string of
    /// numbers. The relay settles on an address and seven ports, and only the address is worth
    /// repeating: "8890 · 8554 · 8889" beside a step name says less than a blank does, because
    /// a port means nothing without the label it sat under.
    /// </summary>
    public string Shorthand(string groupKey, Settings? settings)
    {
        if (settings is null)
        {
            return "";
        }

        var publish = settings.Publish;
        return groupKey switch
        {
            "stream" => publish.Name,
            "source" => Join(Words.Capture(publish.Capture), Picture(publish)),
            "quality" => Join(CodecShorthand(publish.Codec), Quality(publish)),
            "audio" => publish.Audio is "" or "none"
                ? "No audio"
                : Join(Words.AudioSource(publish.Audio), Words.AudioCodec(publish.AudioCodec)),
            "transport" => Words.Transport(publish.PublishTransport),
            // The watch group holds two legs now, and the shorthand names the player's:
            // it is the one a reader acts on from the roster, where the tile's leg is what
            // a tile opens for itself.
            "watch" => Words.Transport(settings.Viewer.PlayerWatchTransport),
            "relay" => settings.Relay.Host,
            _ => "",
        };
    }

    /// <summary>
    /// The whole configuration in one line: what quality it holds, and on what picture.
    /// The two answer the questions a reader glancing at a running stream has, in that
    /// order, because the picture is the part they can see for themselves.
    /// </summary>
    public string Headline(PublishSettings? settings) =>
        settings is null ? "" : Join(Quality(settings), Picture(settings));

    /// <summary>
    /// What the rate control settled on. Each mode names the number it is actually holding,
    /// because that number is the answer and the mode's name alone is not: "20 Mbit/s" is
    /// what a reader checks against their connection.
    /// </summary>
    private static string Quality(PublishSettings s) => s.Mode switch
    {
        "crf" => $"quality {s.Cq}",
        "cbr" => $"{s.BitrateMbps} Mbit/s fixed",
        "abr" => $"{s.BitrateMbps} Mbit/s average",
        "vbr" => $"{s.BitrateMbps}-{s.MaxrateMbps} Mbit/s",
        "lossless" => "lossless",
        _ => s.Mode,
    };

    /// <summary>
    /// The picture, in the shorthand every viewer already reads: "1080p60". The size is the
    /// one being sent, which is the scaled size where one is set and the screen's own where
    /// it is not - and where neither is known, the frame rate stands alone rather than the
    /// line claiming a size nothing measured.
    /// </summary>
    private string Picture(PublishSettings s)
    {
        var height = Height(s);
        return height > 0 ? $"{height}p{s.Fps}" : $"{s.Fps} fps";
    }

    /// <summary>The height being sent: the scaled one, or the captured screen's own.</summary>
    private int Height(PublishSettings s)
    {
        if (s.OutputResolution.Length > 0)
        {
            var parts = s.OutputResolution.Split('x');
            if (parts.Length == 2 && int.TryParse(parts[1], NumberStyles.None, CultureInfo.InvariantCulture, out var scaled))
            {
                return scaled;
            }
        }

        return Monitor(s.Monitor.ToString(CultureInfo.InvariantCulture))?.Height ?? 0;
    }

    /// <summary>
    /// A codec named by what it produces and what produces it, which are the two questions
    /// it answers: the format is what a viewer has to decode and the family is what this
    /// machine has to have. Without the catalog the encoder's own name stands, which is
    /// what the backend called it and what a log line will spell.
    ///
    /// Where a family holds more than one encoder for a format, those two facts no longer
    /// separate the entries and the encoder's own name is appended. It is the software AV1
    /// encoders that this happens to, and their names are what the command preview prints
    /// and what the paragraph behind each entry is about.
    /// </summary>
    private string Codec(string name)
    {
        var row = Row(name);
        if (row is null)
        {
            return name;
        }

        var named = $"{Words.Format(row.Format)} · {Words.Family(row.Family)}";
        return SharesFormatAndFamily(row) ? $"{named} · {name}" : named;
    }

    /// <summary>
    /// Whether another row of the catalog produces this row's format on its family, which
    /// is what decides that neither name identifies an encoder on its own.
    /// </summary>
    private bool SharesFormatAndFamily(VideoCodec row)
    {
        if (_catalog is null)
        {
            return false;
        }

        foreach (var other in _catalog.Codecs)
        {
            if (other.Name != row.Name && other.Format == row.Format && other.Family == row.Family)
            {
                return true;
            }
        }

        return false;
    }

    /// <summary>
    /// A capture backend named by what it reads and by the engine that reads it. The engine
    /// is half the choice rather than a footnote: it decides which encoders and which
    /// transports the rest of the form goes on to offer, and it is what a refusal elsewhere
    /// tells the reader to change - "pick a capture method that runs ffmpeg" names nothing
    /// a reader can find in a list that does not say which is which.
    ///
    /// Without the catalog, and for an entry newer than its rows, the source's name stands
    /// alone.
    /// </summary>
    private string Capture(string value)
    {
        var engine = Words.Engine(CaptureRow(value)?.Engine ?? Api.V1.Engine.Unspecified);
        return engine.Length > 0 ? $"{Words.Capture(value)} · {engine}" : Words.Capture(value);
    }

    private CaptureBackend? CaptureRow(string value)
    {
        if (_catalog is null || value.Length == 0)
        {
            return null;
        }

        foreach (var capture in _catalog.Captures)
        {
            if (capture.Name == value)
            {
                return capture;
            }
        }

        return null;
    }

    /// <summary>The same two facts in the shorthand a step chip has room for.</summary>
    private string CodecShorthand(string name)
    {
        var row = Row(name);
        return row is null ? name : Words.Format(row.Format);
    }

    /// <summary>
    /// What a codec is, which is the format's paragraph plus whatever the encoder adds
    /// beyond it. Only the three software AV1 encoders add anything: the format does not
    /// identify them and they differ enough to choose between.
    /// </summary>
    private string DescribeCodec(string name)
    {
        var row = Row(name);
        if (row is null)
        {
            return "";
        }

        var lines = new List<string> { Descriptions.Format(row.Format), Descriptions.Family(row.Family) };
        var encoder = Descriptions.Encoder(name);
        if (encoder.Length > 0)
        {
            lines.Add(encoder);
        }

        return string.Join("\n", lines.Where(line => line.Length > 0));
    }

    /// <summary>
    /// A pixel format, with the identifier kept beside the plain-language half. The reader
    /// will meet <c>yuv420p</c> again in the command preview and in every answer they find
    /// elsewhere, so hiding it would make this app the only place it has another name.
    /// </summary>
    private static string Chroma(string value)
    {
        var word = Words.Chroma(value);
        return word == value ? value : $"{value} · {word}";
    }

    /// <summary>
    /// A screen, named by its index and what it can show. The index leads because it is
    /// what the reader matches against their own arrangement and what the settings carry.
    /// </summary>
    private string Screen(string value)
    {
        var monitor = Monitor(value);
        if (monitor is null)
        {
            return $"Screen {value}";
        }

        var name = $"Screen {monitor.Index} · {monitor.Width} × {monitor.Height}";
        if (monitor.HasRefreshHz)
        {
            name += $" · {monitor.RefreshHz} Hz";
        }

        return monitor.Primary ? name + " · main" : name;
    }

    /// <summary>
    /// An output size. The empty value is the capture's own size, which is the entry that
    /// scales nothing.
    /// </summary>
    private static string Resolution(string value)
    {
        if (value.Length == 0)
        {
            return "Same as the screen";
        }

        var parts = value.Split('x');
        return parts.Length == 2 ? $"{parts[0]} × {parts[1]}" : value;
    }

    /// <summary>
    /// A step of the frame rate's ladder, named with its unit.
    ///
    /// The unit is repeated here although the field already states it beside its label,
    /// because these entries are read where that label is not: an opened menu is a list of
    /// bare figures otherwise, and "60" and "120" say what they are only to a reader who
    /// still has the heading in view.
    /// </summary>
    private static string Rate(string value) => $"{value} fps";

    /// <summary>What scaling costs and buys, said once for every scaled entry.</summary>
    private static string Scaling(string value) => value.Length == 0
        ? ""
        : "Costs sharpness and saves everything downstream at once: fewer bits to encode, to upload and for your "
          + "viewers to decode.";

    private VideoCodec? Row(string name)
    {
        if (_catalog is null || name.Length == 0)
        {
            return null;
        }

        foreach (var codec in _catalog.Codecs)
        {
            if (codec.Name == name)
            {
                return codec;
            }
        }

        return null;
    }

    private Api.V1.Monitor? Monitor(string value)
    {
        if (_catalog is null || !int.TryParse(value, NumberStyles.None, CultureInfo.InvariantCulture, out var index))
        {
            return null;
        }

        foreach (var monitor in _catalog.Monitors)
        {
            if (monitor.Index == index)
            {
                return monitor;
            }
        }

        return null;
    }

    /// <summary>
    /// A shorthand out of its parts, dropping the ones that had nothing to say. An empty
    /// part joined anyway would leave a separator with nothing on one side of it, which
    /// reads as something having failed to render.
    /// </summary>
    private static string Join(params string[] parts) =>
        string.Join(" · ", parts.Where(part => part.Length > 0));
}
