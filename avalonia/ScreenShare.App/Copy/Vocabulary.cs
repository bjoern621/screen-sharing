using System.Globalization;
using ScreenShare.Api.V1;

namespace ScreenShare.App.Copy;

/// <summary>
/// How this shell names one option of one control, and what it says about it.
///
/// The field key is the switch and the value the lookup, a value alone not identifying an entry: <c>auto</c>
/// under the frame path and <c>auto</c> under the download route are different entries.
/// A control added to the contract is a row here and a change nowhere else.
///
/// Three entries need more than their own value and all three read the catalog: a codec takes the format and
/// family its row carries, a capture backend the engine that reads it, and a monitor the size and refresh rate of
/// the output at that index.
/// The catalog is absent until the first read lands, so every method answers without one: a codec falls back to
/// its encoder name and a screen to its index, which are what the backend called them.
///
/// Two of the three are named against the whole table rather than off their own row, a name that repeats not
/// identifying: one screen is read by both engines, and one format is produced by several encoders in a family.
/// </summary>
public sealed class Vocabulary
{
    /// <summary>Used before the catalog arrives. Every method answers, some more briefly.</summary>
    public static readonly Vocabulary Empty = new(null);

    private readonly Catalog? _catalog;

    public Vocabulary(Catalog? catalog) => _catalog = catalog;

    /// <summary>What one entry of one control is called, at the width of a dropdown row.</summary>
    public string Name(string fieldKey, string value) => Fields.Template(fieldKey) switch
    {
        "publish.capture" => Capture(value),
        "publish.monitor" => Screen(value),
        "publish.output_resolution" => Resolution(value),
        "publish.fps" => Rate(value),
        "publish.maxrate_mbps" => Ceiling(value),
        "publish.capture_memory" => Words.Memory(value),
        "publish.drm_map" => Words.DrmMap(value),
        "publish.codec" => Codec(value),
        "publish.chroma" => Chroma(value),
        "publish.color_range" => Words.ColorRange(value),
        "publish.effort" => Words.Effort(value),
        "publish.tune" => Words.Tune(value),
        "publish.mode" => Words.Mode(value),
        "publish.audio_sources[].source" => Words.AudioSource(value),
        "publish.audio_sources[].device" => Device(value),
        "publish.audio_codec" => Words.AudioCodec(value),
        "publish.publish_transport" or "viewer.tile_watch_transport" => Words.Transport(value),
        "publish.rtsp_publish_protocol" or "viewer.rtsp_watch_protocol" => Words.RtspProtocol(value),
        "viewer.render_chain" => Words.RenderChain(value),
        _ => value,
    };

    /// <summary>
    /// Paragraph behind one entry, empty where the name says it all.
    /// What a radio card prints under its title, and a dropdown where there is room.
    /// </summary>
    public string Describe(string fieldKey, string value) => Fields.Template(fieldKey) switch
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
        "publish.audio_sources[].source" => Descriptions.AudioSource(value),
        "publish.audio_codec" => Descriptions.AudioCodec(value),
        "publish.publish_transport" or "viewer.tile_watch_transport" => Descriptions.Transport(value),
        "publish.rtsp_publish_protocol" or "viewer.rtsp_watch_protocol" => Descriptions.RtspProtocol(value),
        "viewer.render_chain" => Descriptions.RenderChain(value),
        _ => "",
    };

    /// <summary>
    /// What one group settled on, in the few words a step chip repeats.
    /// Composed here rather than received: the separator, the abbreviation and the length are the strip's
    /// decisions.
    /// The values behind it are the draft's, so it says nothing the form does not.
    /// A group with nothing worth a line answers with nothing: the relay settles on an address and seven ports,
    /// and "8890 · 8554 · 8889" beside a step name says less than a blank, a port meaning nothing without the
    /// label it sat under.
    /// </summary>
    public string Shorthand(FieldGroup? group, Settings? settings)
    {
        if (group is null || settings is null)
        {
            return "";
        }

        var publish = settings.Publish;
        return group.Key switch
        {
            "stream" => publish.Name,
            "source" => Join(Words.Capture(publish.Capture), Picture(publish)),
            "quality" => Join(CodecShorthand(publish.Codec), Quality(publish, CeilingOf(group))),
            "audio" => AudioShorthand(publish),
            "transport" => Words.Transport(publish.PublishTransport),
            // The tile's leg, the group's only one: a player and a browser page take theirs from the roster, per
            // press.
            "watch" => Words.Transport(settings.Viewer.TileWatchTransport),
            "relay" => settings.Relay.Host,
            _ => "",
        };
    }

    /// <summary>
    /// What the rate control settled on, with the number each mode is holding.
    /// The mode's name alone is not the answer: "20 Mbit/s" is what a reader checks against their connection.
    /// A quality target holds no number a line can be read against, so it carries the rate it is held to where
    /// there is one: <c>quality 18, up to 45 Mbit/s</c>.
    /// </summary>
    private static string Quality(PublishSettings s, double? ceilingMbps) => s.Mode switch
    {
        "crf" => ceilingMbps is { } ceiling
            ? $"quality {s.Cq}, up to {ceiling:0} Mbit/s"
            : $"quality {s.Cq}",
        "cbr" => $"{s.BitrateMbps} Mbit/s fixed",
        "abr" => $"{s.BitrateMbps} Mbit/s average",
        "vbr" => $"{s.BitrateMbps}-{s.MaxrateMbps} Mbit/s",
        "lossless" => "lossless",
        _ => s.Mode,
    };

    /// <summary>
    /// Rate this encode is held to, null where it is held to none.
    /// Read off the ceiling control the backend offered rather than off the settings value beside it: whether a
    /// ceiling reaches the encoder is the mode's and the element's answer, and the form states it by offering the
    /// field or greying it (<c>docs/field-availability.md</c>).
    /// A control the form withheld therefore says nothing here, which is what keeps the line from naming a bound
    /// nothing is holding.
    /// </summary>
    private static double? CeilingOf(FieldGroup group)
    {
        foreach (var field in group.Fields)
        {
            if (field.Key != "publish.maxrate_mbps" || !field.Enabled)
            {
                continue;
            }

            return field.Value?.Number > 0 ? field.Value.Number : null;
        }

        return null;
    }

    /// <summary>
    /// Picture in the shorthand every viewer already reads: "1080p60".
    /// The size is the one going out, scaled where a scale is set and the screen's own where it is not.
    /// Where neither is known the frame rate stands alone, rather than the line claiming a size nothing measured.
    /// </summary>
    private string Picture(PublishSettings s)
    {
        var height = Height(s);
        return height > 0 ? $"{height}p{s.Fps}" : $"{s.Fps} fps";
    }

    /// <summary>
    /// What the second track is mixed from, at a step chip's width.
    /// One source is named and several are counted, a chip listing three kinds being one nobody reads at a
    /// glance.
    /// An entry recording nothing is not a source: the list carries a row for a reader to grow it by, and one
    /// turned off keeps its place until the next resolve takes it away.
    /// </summary>
    private string AudioShorthand(PublishSettings s)
    {
        var recording = s.AudioSources.Where(a => a.Source is not ("" or "none")).ToList();
        return recording.Count switch
        {
            0 => "No audio",
            1 => Join(Words.AudioSource(recording[0].Source), Words.AudioCodec(s.AudioCodec)),
            _ => Join($"{recording.Count} sources", Words.AudioCodec(s.AudioCodec)),
        };
    }

    /// <summary>Height going out: a set scale, else the captured screen's.</summary>
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
    /// A codec named by what it produces and what produces it: the format a viewer has to decode, and the family
    /// this machine has to have.
    /// Without the catalog the encoder's own name stands, which is what a log line spells.
    /// Where a family holds more than one encoder for a format, those two facts stop separating the entries and
    /// the encoder's name is appended. The software AV1 encoders are the ones this happens to.
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
    /// A capture backend named by what it reads and by the engine reading it.
    /// The engine is half the choice: it decides which encoders and transports the rest of the form offers, and
    /// it is what a refusal elsewhere asks the reader to change.
    /// Without the catalog, and for a source newer than its rows, the source name stands alone.
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

    /// <summary>Format alone, at a step chip's width.</summary>
    private string CodecShorthand(string name)
    {
        var row = Row(name);
        return row is null ? name : Words.Format(row.Format);
    }

    /// <summary>
    /// Format's paragraph, plus whatever the encoder adds beyond it.
    /// Only the software AV1 encoders add anything: the format does not identify them, and they differ enough to
    /// choose between.
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
    /// A pixel format, the identifier kept beside the plain-language half.
    /// <c>yuv420p</c> is what the command preview prints and what every answer found elsewhere calls it, so
    /// dropping it would make this app the one place it has another name.
    /// </summary>
    private static string Chroma(string value)
    {
        var word = Words.Chroma(value);
        return word == value ? value : $"{value} · {word}";
    }

    /// <summary>
    /// A screen, named by its index and what it can show.
    /// The index leads: what the settings carry, and what a reader matches against their own arrangement.
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
    /// One device inside an audio kind.
    /// The empty value is the kind's default, following whatever the system is set to.
    /// Anything else is named off the catalog's enumeration and falls back to the handle: a device the
    /// enumeration no longer reports is one the settings still carry, and naming what a publish would open beats
    /// a blank.
    /// </summary>
    private string Device(string value)
    {
        if (value.Length == 0)
        {
            return "System default";
        }

        var device = _catalog?.AudioDevices.FirstOrDefault(d => d.Id == value);
        return device is null || device.Name.Length == 0 ? value : device.Name;
    }

    /// <summary>
    /// An output size.
    /// The empty value is the capture's own size, the entry that scales nothing.
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
    /// A step of the frame-rate ladder, carrying its unit.
    /// The field states the unit beside its label, but an opened menu is read away from that label, where "60"
    /// and "120" say what they are only to a reader still holding the heading in view.
    /// </summary>
    private static string Rate(string value) => $"{value} fps";

    /// <summary>
    /// A burst ceiling, whose zero is not a rate but the absence of one.
    /// The unit rides beside the control, so a figure carries none of its own.
    /// </summary>
    private static string Ceiling(string value) => value == "0" ? "No ceiling" : value;

    /// <summary>What scaling trades, said once for every entry that scales.</summary>
    private static string Scaling(string value) => value.Length == 0
        ? ""
        : "Costs sharpness and saves everything downstream at once: fewer bits to encode, to upload and for "
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
    /// A shorthand out of its parts, dropping the ones with nothing to say.
    /// An empty part joined anyway leaves a separator with nothing on one side, reading as a failure to render.
    /// </summary>
    private static string Join(params string[] parts) =>
        string.Join(" · ", parts.Where(part => part.Length > 0));
}
