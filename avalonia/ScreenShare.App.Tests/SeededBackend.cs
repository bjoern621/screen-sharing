using System.Runtime.CompilerServices;
using Google.Protobuf.Reflection;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Tests;

/// <summary>
/// A <see cref="Form"/> to render, without a backend behind it. This is the fixture the seam
/// exists for: the setup flow can be driven by something other than a running service, so its
/// behaviour is stated in a test rather than in a screenshot.
///
/// <b>It lives in the test project, and that placement is the point.</b> It was the shipped
/// shell's stand-in until <see cref="ControlBackend"/> answered over the local socket, and it
/// is the one file that names a codec: a greying written here is a rule written twice, which
/// is exactly what <c>docs/ipc-api.md</c> exists to prevent. In the app that would be a
/// defect. In a test it is a fixture, and it says what a form looks like rather than what the
/// domain is.
///
/// Everything here is therefore a seed rather than a rule. The values, labels and refusal
/// sentences are taken from the Go tables they are really computed from -
/// <c>capabilities.Codecs</c>, <c>gpupath.Paths</c>, the transport registry and the platform
/// gates - so a test reads against the product's own vocabulary. What it does not do is
/// evaluate those tables: the greyings below are the few that demonstrate each of the four
/// treatments in <c>docs/field-availability.md</c>, not the full evaluation.
///
/// The one structural rule it keeps is the contract's own: nothing above it learns a codec
/// name, a transport name or a label. They cross as data.
///
/// It answers both reads from memory, so both hand back an already-completed task. What it
/// must not do is block or hand back a null task, because the flow above it is written
/// against the gRPC client's timing and would then never be exercised on the path it really
/// runs.
/// </summary>
internal sealed class SeededBackend : IBackend
{
    /// <summary>
    /// Never raised. A fixture answering from a dictionary has no probe landing behind it and
    /// nothing else that moves, so the accessors are empty rather than backed by a field
    /// nothing would ever invoke.
    /// </summary>
    public event Action? Changed
    {
        add { }
        remove { }
    }

    /// <summary>One option of a seeded select or radio, before the draft decides which is picked.</summary>
    private sealed record OptionSeed
    {
        public required string Value { get; init; }

        /// <summary>What the entry was derived from, and null where it needs no annotation.</summary>
        public Text? Note { get; init; }

        /// <summary>Why this combination rules the option out, and null where it does not.</summary>
        public Text? Reason { get; init; }
    }

    /// <summary>One seeded control, before the draft supplies its value.</summary>
    private sealed record FieldSeed
    {
        /// <summary>The <see cref="StreamSettings"/> field this control edits, named as that message names it.</summary>
        public required string Key { get; init; }

        public required ControlKind Control { get; init; }

        /// <summary>What this control's number means, and unset where it is not a quantity.</summary>
        public Unit Unit { get; init; } = Unit.Unspecified;

        public IReadOnlyList<OptionSeed> Options { get; init; } = [];

        public NumericRange? Range { get; init; }
    }

    /// <summary>One seeded group: a run of fields under a heading, in render order.</summary>
    private sealed record GroupSeed
    {
        public required string Key { get; init; }

        public required IReadOnlyList<FieldSeed> Fields { get; init; }
    }

    /// <summary>
    /// The publish engine each capture backend runs, which is the fact most greyings on
    /// this screen hang off. Seeded from <c>publish.captureBackends</c>.
    /// </summary>
    private static readonly IReadOnlyDictionary<string, string> EngineOf = new Dictionary<string, string>
    {
        ["ddagrab"] = "ffmpeg",
        ["gdigrab"] = "ffmpeg",
        ["x11grab"] = "ffmpeg",
        ["kmsgrab"] = "ffmpeg",
        ["avfoundation"] = "ffmpeg",
        ["d3d11screencapturesrc"] = "gstreamer",
        ["ximagesrc"] = "gstreamer",
        ["portal"] = "gstreamer",
        ["avfvideosrc"] = "gstreamer",
    };

    /// <summary>The operating system each capture backend needs, and the sentence shown elsewhere.</summary>
    private static readonly IReadOnlyDictionary<string, (string Os, string WrongOs)> PlatformOf =
        new Dictionary<string, (string Os, string WrongOs)>
        {
            ["ddagrab"] = ("windows", "DXGI Desktop Duplication is Windows-only"),
            ["gdigrab"] = ("windows", "GDI capture is Windows-only"),
            ["d3d11screencapturesrc"] = ("windows", "Direct3D 11 screen capture is Windows-only"),
            ["x11grab"] = ("linux", "X11 capture is Linux-only"),
            ["ximagesrc"] = ("linux", "X11 capture is Linux-only"),
            ["kmsgrab"] = ("linux", "DRM/KMS capture is Linux-only"),
            ["portal"] = ("linux", "PipeWire ScreenCast is Linux-only"),
            ["avfoundation"] = ("darwin", "AVFoundation screen capture is macOS-only"),
            ["avfvideosrc"] = ("darwin", "AVFoundation screen capture is macOS-only"),
        };

    /// <summary>The encoder family behind each codec, for the greyings that follow the backend.</summary>
    private static readonly IReadOnlyDictionary<string, string> FamilyOf = new Dictionary<string, string>
    {
        ["hevc_nvenc"] = "nvenc",
        ["h264_nvenc"] = "nvenc",
        ["av1_nvenc"] = "nvenc",
        ["libx264"] = "software",
        ["libx265"] = "software",
        ["libvpx-vp9"] = "software",
        ["libsvtav1"] = "software",
        ["h264_vaapi"] = "vaapi",
        ["hevc_vaapi"] = "vaapi",
        ["h264_qsv"] = "qsv",
        ["hevc_qsv"] = "qsv",
        ["h264_amf"] = "amf",
    };

    private readonly string _os;

    public SeededBackend(string operatingSystem) => _os = operatingSystem;

    /// <summary>
    /// The settings a first start opens on, answered from memory. Seeded from the Go
    /// <c>settings.Defaults</c>, including its per-platform capture backend.
    /// </summary>
    /// <summary>
    /// The reference set, answered from memory with the two rows a name is composed from:
    /// one codec so a dropdown entry can be named by its format and family, and one screen
    /// so the picture shorthand has a height. The fixture states what the copy reads, and
    /// nothing here decides what is legal.
    /// </summary>
    public Task<Catalog> CatalogAsync(CancellationToken cancellation = default)
    {
        if (cancellation.IsCancellationRequested)
        {
            return Task.FromCanceled<Catalog>(cancellation);
        }

        var catalog = new Catalog();
        catalog.Codecs.Add(new VideoCodec
        {
            Name = "libx264",
            Family = "software",
            Format = "h264",
            Implemented = true,
        });
        catalog.Monitors.Add(new global::ScreenShare.Api.V1.Monitor { Index = 0, Width = 2560, Height = 1440, RefreshHz = 144, Primary = true });
        return Task.FromResult(catalog);
    }

    public Task<StreamSettings> SettingsAsync(CancellationToken cancellation = default)
    {
        // Honoured rather than ignored: a caller that has already abandoned this read
        // gets the answer the socket would have given it, so the cancellation path is
        // the same one whichever implementation is behind the seam.
        return cancellation.IsCancellationRequested
            ? Task.FromCanceled<StreamSettings>(cancellation)
            : Task.FromResult(Defaults());
    }

    private StreamSettings Defaults() => new()
    {
        Name = Environment.MachineName.Length > 0 ? Environment.MachineName : "me",
        RelayHost = "127.0.0.1",
        RelayPort = 8890,
        ApiPort = 9997,
        RtspPort = 8554,
        WebrtcPort = 8889,
        RtmpPort = 1935,
        HlsPort = 8888,
        MoqPort = 8892,
        Transport = "srt",
        Codec = "hevc_nvenc",
        Mode = "lossless",
        Chroma = "gbrp",
        ColorRange = "pc",
        Fps = 60,
        Cq = 19,
        BitrateMbps = 150,
        MaxrateMbps = 200,
        VbvMs = 0,
        Gop = 0,
        Bframes = 0,
        EncPreset = "p7",
        Capture = _os == "windows" ? "ddagrab" : _os == "darwin" ? "avfoundation" : "x11grab",
        Audio = "none",
        AudioCodec = "opus",
        DrmMap = "auto",
        Monitor = 0,
        CaptureMemory = "auto",
        SrtPublishLatencyMs = 300,
        SrtWatchLatencyMs = 1200,
        RtspPublishProtocol = "tcp",
        RtspWatchProtocol = "tcp",
        UplinkMbps = 50,
        WatchTransport = "srt",
        OutputResolution = "",
    };

    /// <summary>
    /// Resolves one draft into the screen, answered from memory. Repairs nothing, so
    /// <see cref="Form.RepairedFieldKeys"/> is always empty: a stand-in that walked the tables
    /// to a legal value would be walking tables it does not have.
    /// </summary>
    public Task<Form> ResolveFormAsync(StreamSettings draft, CancellationToken cancellation = default)
    {
        Assert.NotNull(draft, "resolving a form needs the draft it is resolved against");

        return cancellation.IsCancellationRequested
            ? Task.FromCanceled<Form>(cancellation)
            : Task.FromResult(Resolve(draft));
    }

    // --- The rest of the seam -------------------------------------------------------
    //
    // This fixture exists for the setup flow, which is the surface with the cross-field rules
    // worth stating in a test. The running state is not seeded: there is no pipeline behind
    // this, no relay and no child process, so each of these answers the honest version of that
    // rather than a plausible-looking figure.
    //
    // They are here rather than on a second stand-in because IBackend is one seam. A partial
    // implementation would make every test that touches a new method fail to compile for a
    // reason that has nothing to do with what it is testing - which is exactly what happened
    // to this file once already.

    /// <summary>Nothing publishes. The absent <c>Live</c> is what says so.</summary>
    public Task<PublishState> PublishStateAsync(CancellationToken cancellation = default)
        => Task.FromResult(new PublishState());

    /// <summary>
    /// No relay answered, carrying the reason. An unreachable relay is a snapshot and never a
    /// failure, so a screen driven by this fixture renders the sentence rather than an error.
    /// </summary>
    public Task<RelayStatus> RelayStatusAsync(CancellationToken cancellation = default)
        => Task.FromResult(new RelayStatus
        {
            Reachable = false,
            Error = "no relay behind this shell yet",
        });

    /// <summary>No viewer is open, which is an empty list rather than an absent one.</summary>
    public Task<IReadOnlyList<WatchKey>> WatchingAsync(CancellationToken cancellation = default)
        => Task.FromResult<IReadOnlyList<WatchKey>>([]);

    public Task StartPublishAsync(StreamSettings settings, CancellationToken cancellation = default)
    {
        Assert.NotNull(settings, "starting a publish names the settings the encoder runs on");

        return Task.CompletedTask;
    }

    public Task StopPublishAsync(CancellationToken cancellation = default) => Task.CompletedTask;

    /// <summary>
    /// A measurement nothing measured. There is no socket behind this fixture, so it answers
    /// a fixed figure a test can assert against rather than a plausible-looking random one.
    /// </summary>
    public Task<double> MeasureUplinkAsync(CancellationToken cancellation = default)
        => Task.FromResult(MeasuredUplinkMbps);

    /// <summary>What <see cref="MeasureUplinkAsync"/> answers, so a test can name it.</summary>
    public const double MeasuredUplinkMbps = 87;

    public Task StartWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
        => Task.CompletedTask;

    public Task StopWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
        => Task.CompletedTask;

    public Task OpenLogAsync(string path, CancellationToken cancellation = default) => Task.CompletedTask;

    public Task OpenLogsFolderAsync(CancellationToken cancellation = default) => Task.CompletedTask;

    /// <summary>
    /// A stream that ends at once. Nothing behind this fixture changes on its own, so there is
    /// no event to deliver; ending is what the real client reads as the backend going away, and
    /// it is preferable to a stream that never yields and never returns.
    /// </summary>
    public async IAsyncEnumerable<Event> SubscribeAsync(
        [EnumeratorCancellation] CancellationToken cancellation = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        yield break;
    }

    /// <summary>The whole of the resolve, with no wire in front of it.</summary>
    private Form Resolve(StreamSettings draft)
    {
        var settings = draft.Clone();
        var form = new Form { Settings = settings, Publishable = true };

        foreach (var group in Groups())
        {
            form.Groups.Add(Resolve(group, settings));
        }

        form.Summary = new Summary
        {
            Command = "",
            CommandError = "no backend behind this shell yet, so no command was rendered",
            Estimate = new Estimate { BitrateMbps = 11.8, RawMbps = 3110.4, HeadroomMbps = settings.UplinkMbps - 11.8 },
        };

        if (settings.UplinkMbps < form.Summary.Estimate.BitrateMbps)
        {
            form.Diagnostics.Add(new Diagnostic
            {
                Severity = Severity.Warning,
                FieldKey = "uplink_mbps",
                Text = Say(TextCode.UplinkBelowPrediction,
                    Dec(TextArgName.BitrateMbps, form.Summary.Estimate.BitrateMbps),
                    Num(TextArgName.UplinkMbps, settings.UplinkMbps)),
            });
        }

        Assert.That(form.Groups.Count == Groups().Count, "a resolved group per seeded group", form.Groups.Count);
        return form;
    }

    /// <summary>
    /// One group with its draft-dependent parts filled in: each field's value, its
    /// visibility and enabled state, and which option is picked.
    /// </summary>
    private FieldGroup Resolve(GroupSeed seed, StreamSettings settings)
    {
        var group = new FieldGroup { Key = seed.Key };

        foreach (var field in seed.Fields)
        {
            group.Fields.Add(Resolve(field, settings));
        }

        return group;
    }

    private Field Resolve(FieldSeed seed, StreamSettings settings)
    {
        var (visible, enabled, reason, note) = Availability(seed.Key, settings);

        var field = new Field
        {
            Key = seed.Key,
            Control = seed.Control,
            Unit = seed.Unit,
            Visible = visible,
            Enabled = enabled,
            Reason = reason,
            Note = note,
            Value = ValueOf(seed.Key, settings),
            Range = seed.Range,
        };

        var picked = Picked(field.Value);
        foreach (var option in seed.Options)
        {
            var refusal = option.Reason ?? OptionRefusal(seed.Key, option.Value, settings);
            field.Options.Add(new FieldOption
            {
                Value = option.Value,
                Note = option.Note,
                Enabled = refusal is null,
                Reason = refusal,
                Recommended = false,
            });
        }

        Assert.That(
            field.Options.Count == 0 || field.Options.Any(option => option.Value == picked),
            "a select field's value is one of the options it offers", seed.Key, picked);
        return field;
    }

    /// <summary>
    /// The value the picked option carries, for the assertion above. It is the option's own
    /// string form whatever the settings field's type is, so a select over a number is held
    /// to the same invariant as one over a name.
    /// </summary>
    private static string Picked(FieldValue value) => FieldValues.AsText(value);

    /// <summary>
    /// The field's current value, read off the draft through the descriptor rather than a
    /// switch. The key is the settings field's own name, which is what makes that possible
    /// and what the contract relies on everywhere else.
    /// </summary>
    private static FieldValue ValueOf(string key, StreamSettings settings)
    {
        var descriptor = StreamSettings.Descriptor.FindFieldByName(key);
        Assert.NotNull(descriptor, "a form field names a settings field");

        var raw = descriptor.Accessor.GetValue(settings);
        return descriptor.FieldType switch
        {
            FieldType.String => new FieldValue { Text = (string)raw },
            FieldType.Int32 => new FieldValue { Number = (int)raw },
            FieldType.Int64 => new FieldValue { Number = (long)raw },
            FieldType.Bool => new FieldValue { Flag = (bool)raw },
            FieldType.Double => new FieldValue { Decimal = (double)raw },
            _ => Assert.Never<FieldValue>("a form field's settings type is one the contract carries", key, descriptor.FieldType),
        };
    }

    /// <summary>
    /// The four treatments, seeded. Each entry here mirrors a rule that lives in the Go
    /// tables today and will be evaluated there: a hidden backend knob, a disabled general
    /// concept with the reason the reader can act on, and a live field with a note.
    /// </summary>
    private (bool Visible, bool Enabled, Text? Reason, Text? Note) Availability(string key, StreamSettings settings)
    {
        switch (key)
        {
            // Hidden: a knob of the kmsgrab scanout path and of nothing else, whose help
            // text would teach a reader on another backend nothing.
            case "drm_map":
                return (settings.Capture == "kmsgrab", true, null, null);

            // Disabled with a reason: a general encoding concept this combination blocks.
            // Where two facts block it, the reason names the one the reader can act on -
            // the family before the engine, since another codec is nearer to hand.
            case "enc_preset":
                if (FamilyOf.GetValueOrDefault(settings.Codec, "") != "nvenc")
                {
                    return (true, false, Say(TextCode.PresetOnlyOnFamilies, Ids(TextArgName.Families, "nvenc")), null);
                }
                if (EngineOf.GetValueOrDefault(settings.Capture, "") == "gstreamer")
                {
                    return (true, false, Say(TextCode.GstNoPresetLadder), null);
                }
                return (true, true, null, null);

            case "audio_codec":
                return settings.Audio == "none"
                    ? (true, false, Say(TextCode.AudioCodecNeedsSource), null)
                    : (true, true, null, null);

            case "srt_publish_latency_ms":
                return (settings.Transport == "srt", true, null, null);

            case "rtsp_publish_protocol":
                return (settings.Transport == "rtsp", true, null, null);

            // Disabled with a reason, from the mode rather than from the codec: only the
            // constant-quality mode aims at a quality, so the modes that aim at a bitrate
            // grey the quantizer and name the mode, which is the fact the reader can act on.
            case "cq":
                return settings.Mode == "crf"
                    ? (true, true, null, null)
                    : (true, false, Say(TextCode.CqOnlyInConstantQuality), null);

            // Live with a note: the value still reaches the encoder and means something the
            // heading does not describe here.
            case "monitor":
                return (true, true, null, Say(TextCode.MonitorNotEnumerated, Num(TextArgName.Monitor, settings.Monitor)));

            default:
                return (true, true, null, null);
        }
    }

    /// <summary>
    /// Why one option of one field is ruled out here. The seeded set covers the three
    /// kinds the tables produce: a platform gate, a pair with no device path, and an
    /// engine that lacks the element.
    /// </summary>
    private Text? OptionRefusal(string key, string value, StreamSettings settings)
    {
        var engine = EngineOf.GetValueOrDefault(settings.Capture, "");

        switch (key)
        {
            case "capture":
                if (!PlatformOf.TryGetValue(value, out var platform) || platform.Os == _os)
                {
                    return null;
                }
                return Say(TextCode.CaptureWrongOs, Id(TextArgName.Capture, value), Id(TextArgName.Os, platform.Os));

            case "audio":
                return value == "desktop" && _os != "linux"
                    ? Say(TextCode.AudioSourceUnserved, Id(TextArgName.Audio, value), Id(TextArgName.Os, _os))
                    : null;

            case "capture_memory":
                if (value is not ("gpu" or "gpu-encoder-color"))
                {
                    // Auto and the system copy are never greyed: auto answers with whichever
                    // path the pair has, and the copy is the path every pair has.
                    return null;
                }
                return HasDevicePath(engine, settings.Capture, FamilyOf.GetValueOrDefault(settings.Codec, ""))
                    ? null
                    : Say(
                        TextCode.PairHasNoDeviceMemory,
                        Id(TextArgName.Capture, settings.Capture),
                        Id(TextArgName.Codec, settings.Codec),
                        Id(TextArgName.Engine, engine));

            case "transport":
                return value == "rtmp" && engine == "gstreamer"
                    ? Say(
                        TextCode.EngineHasNoPublishSink,
                        Id(TextArgName.Capture, settings.Capture),
                        Id(TextArgName.Engine, engine),
                        Id(TextArgName.Transport, value))
                    : null;

            case "codec":
                return engine == "gstreamer" && FamilyOf.GetValueOrDefault(value, "") == "amf"
                    ? Say(TextCode.GapGstAmfcodecWindowsOnly)
                    : null;

            case "chroma":
                // Seeded from the two chroma facts that hold for every codec in the list:
                // 4:2:2 is the software H.26x rows' alone, and direct RGB needs a codec
                // whose encoder takes a GBR input.
                var family = FamilyOf.GetValueOrDefault(settings.Codec, "");
                if (value == "yuv422p" && family != "software")
                {
                    return Say(
                        TextCode.CodecCannotEncodeChroma,
                        Id(TextArgName.Codec, settings.Codec),
                        Id(TextArgName.Chroma, value));
                }
                if (value == "gbrp" && settings.Codec is not ("hevc_nvenc" or "libx265" or "libvpx-vp9"))
                {
                    return Say(TextCode.CodecCodesNoRgb, Id(TextArgName.Codec, settings.Codec));
                }
                return null;

            default:
                return null;
        }
    }

    // --- Building a statement ---------------------------------------------------------
    //
    // The backend states a fact as a code and the identifiers it is about, so a fixture
    // standing in for one builds the same shape. These four are what that takes, and they
    // are the whole of the fixture's vocabulary: no sentence is written here, because a
    // sentence written here would be a second answer beside the shell's own.

    private static Text Say(TextCode code, params TextArg[] args)
    {
        var text = new Text { Code = code };
        text.Args.AddRange(args);
        return text;
    }

    private static TextArg Id(TextArgName name, string id) => new() { Name = name, Id = id };

    private static TextArg Num(TextArgName name, long value) => new() { Name = name, Number = value };

    private static TextArg Dec(TextArgName name, double value) => new() { Name = name, Decimal = value };

    private static TextArg Ids(TextArgName name, params string[] ids)
    {
        var list = new IdList();
        list.Ids.AddRange(ids);
        return new TextArg { Name = name, Ids = list };
    }

    /// <summary>The pairs with a device path, seeded from <c>gpupath.Paths</c>.</summary>
    private static bool HasDevicePath(string engine, string capture, string family) =>
        (engine, capture, family) switch
        {
            ("gstreamer", "portal", "vaapi") => true,
            ("gstreamer", "d3d11screencapturesrc", "nvenc") => true,
            ("ffmpeg", "kmsgrab", "vaapi") => true,
            ("ffmpeg", "ddagrab", "qsv") => true,
            ("ffmpeg", "ddagrab", "nvenc") => true,
            _ => false,
        };

    private static NumericRange Bounded(int min, int max, int step = 1) => new() { Min = min, Max = max, Step = step };

    /// <summary>
    /// The capture source the three quality ladders are derived from, standing in for the
    /// monitor <c>display.List</c> reports. Seeded from the mockups, and the numbers are the
    /// mockups' own.
    ///
    /// It is one record rather than three lists because the lists are consequences of it: a
    /// resolution ladder is the source scaled by whole steps and a frame-rate list is bounded
    /// by what the panel refreshes at. Writing the lists out instead would be writing down an
    /// answer that depends on which monitor is selected.
    /// </summary>
    private static readonly (int Width, int Height, double RefreshHz) Source = (2560, 1440, 59.951);

    /// <summary>The standard heights a source is offered scaled down to, largest first.</summary>
    private static readonly int[] ScaleHeights = [2160, 1440, 1080, 900, 720, 540];

    /// <summary>The frame rates offered, before the source's refresh rate narrows them.</summary>
    private static readonly int[] FrameRates = [24, 30, 50, 60, 120];

    /// <summary>The keyframe intervals offered, in seconds, before the frame rate turns them into counts.</summary>
    private static readonly int[] KeyframeSeconds = [1, 2, 4];

    /// <summary>
    /// The resolutions this source can be scaled to: its own size, and each standard height
    /// below it at the source's aspect ratio. Derived rather than listed, so another monitor
    /// produces another ladder with nothing here edited.
    ///
    /// Every entry carries what it was derived from, which is what makes the control honest:
    /// the reader sees the cost of the choice without opening the step that owns the source.
    /// </summary>
    private static IReadOnlyList<OptionSeed> ResolutionOptions()
    {
        var options = new List<OptionSeed>
        {
            new() { Value = "" },
        };

        var from = new[]
        {
            Num(TextArgName.Width, Source.Width),
            Num(TextArgName.Height, Source.Height),
        };

        foreach (var height in ScaleHeights)
        {
            if (height > Source.Height)
            {
                continue;
            }

            var width = Source.Width * height / Source.Height / 2 * 2;
            options.Add(new OptionSeed
            {
                Value = $"{width}x{height}",
                Note = height == Source.Height ? null : Say(TextCode.ScaledFromSource, from),
            });
        }

        return options;
    }

    /// <summary>
    /// The frame rates, with the ones above the panel's refresh greyed rather than dropped.
    /// A neighbouring source allows them, so the entry stays and says which side the limit is
    /// on (docs/field-availability.md, "One option disabled with a reason").
    /// </summary>
    private static IReadOnlyList<OptionSeed> FrameRateOptions() =>
        [.. FrameRates.Select(rate => new OptionSeed
        {
            // Above the source's own refresh rate the extra frames are repeats, which the
            // form states as a diagnostic rather than as a refusal - so the entry stays
            // offered here, exactly as the tables leave it.
            Value = rate.ToString(),
        })];

    /// <summary>
    /// The keyframe intervals. The settings carry a frame count, so the label is the concept
    /// and the note is the number it works out to: a reader picks two seconds and can still
    /// see it became 120 frames.
    ///
    /// Seeded against the default frame rate rather than the draft's, because a seed that
    /// recomputed the counts per draft would be evaluating a rule instead of standing in for
    /// one. The Go side derives them from the resolved frame rate.
    /// </summary>
    private static IReadOnlyList<OptionSeed> KeyframeOptions()
    {
        var fps = (int)Math.Round(Source.RefreshHz);
        var options = new List<OptionSeed>
        {
            // Auto is not a duration: it is the encoder's own rule, which every builder reads
            // as twice the frame rate. It carries what that works out to, so the entry is
            // measured in the same unit as the ones below it.
            new() { Value = "0" },
        };

        options.AddRange(KeyframeSeconds.Select(seconds => new OptionSeed
        {
            Value = (seconds * fps).ToString(),
        }));

        return options;
    }

    /// <summary>
    /// The seeded screen. Order is render order, and the grouping follows the domain -
    /// what the source is, what encodes it, where it goes - which is why it is stated here
    /// rather than left to a shell to arrange.
    /// </summary>
    private static IReadOnlyList<GroupSeed> Groups() =>
    [
        new()
        {
            Key = "source",
            Fields =
            [
                new()
                {
                    Key = "capture",
                    Control = ControlKind.Radio,
                    Options =
                    [
                        new() { Value = "ddagrab" },
                        new() { Value = "gdigrab" },
                        new() { Value = "d3d11screencapturesrc" },
                        new() { Value = "x11grab" },
                        new() { Value = "ximagesrc" },
                        new() { Value = "kmsgrab" },
                        new() { Value = "portal" },
                        new() { Value = "avfoundation" },
                        new() { Value = "avfvideosrc" },
                    ],
                },
                new()
                {
                    Key = "capture_memory",
                    Control = ControlKind.Select,
                    Options =
                    [
                        new() { Value = "auto" },
                        new() { Value = "gpu" },
                        new() { Value = "gpu-encoder-color" },
                        new() { Value = "system" },
                    ],
                },
                new()
                {
                    Key = "drm_map",
                    Control = ControlKind.Select,
                    Options =
                    [
                        new() { Value = "auto" },
                        new() { Value = "vaapi" },
                        new() { Value = "vulkan" },
                        new() { Value = "none" },
                    ],
                },
                new()
                {
                    Key = "monitor",
                    Control = ControlKind.Number,
                    Range = Bounded(0, 7),
                },
                new()
                {
                    Key = "audio",
                    Control = ControlKind.Select,
                    Options =
                    [
                        new() { Value = "none" },
                        new() { Value = "desktop" },
                    ],
                },
            ],
        },
        new()
        {
            Key = "encoder",
            Fields =
            [
                new()
                {
                    Key = "codec",
                    Control = ControlKind.Select,
                    Options =
                    [
                        new() { Value = "hevc_nvenc" },
                        new() { Value = "h264_nvenc" },
                        new() { Value = "av1_nvenc" },
                        new() { Value = "libx264" },
                        new() { Value = "libx265" },
                        new() { Value = "libvpx-vp9" },
                        new() { Value = "libsvtav1" },
                        new() { Value = "h264_vaapi" },
                        new() { Value = "hevc_vaapi" },
                        new() { Value = "h264_qsv" },
                        new() { Value = "hevc_qsv" },
                        new() { Value = "h264_amf" },
                    ],
                },
                new()
                {
                    Key = "chroma",
                    Control = ControlKind.Select,
                    Options =
                    [
                        new() { Value = "gbrp" },
                        new() { Value = "yuv444p" },
                        new() { Value = "yuv422p" },
                        new() { Value = "yuv420p" },
                        new() { Value = "p010le" },
                    ],
                },
                new()
                {
                    Key = "color_range",
                    Control = ControlKind.Select,
                    Options =
                    [
                        new() { Value = "pc" },
                        new() { Value = "tv" },
                    ],
                },
                new()
                {
                    Key = "enc_preset",
                    Control = ControlKind.Select,
                    Options =
                    [
                        new() { Value = "p1" },
                        new() { Value = "p2" },
                        new() { Value = "p3" },
                        new() { Value = "p4" },
                        new() { Value = "p5" },
                        new() { Value = "p6" },
                        new() { Value = "p7" },
                    ],
                },
                new()
                {
                    Key = "audio_codec",
                    Control = ControlKind.Select,
                    Options =
                    [
                        new() { Value = "opus" },
                        new() { Value = "aac" },
                    ],
                },
            ],
        },
        new()
        {
            Key = "quality",
            Fields =
            [
                new()
                {
                    Key = "mode",
                    Control = ControlKind.Radio,
                    Options =
                    [
                        new() { Value = "crf" },
                        new() { Value = "vbr" },
                        new() { Value = "cbr" },
                        new() { Value = "abr" },
                        new() { Value = "lossless" },
                    ],
                },
                new()
                {
                    Key = "cq",
                    Control = ControlKind.Slider,
                    Range = Bounded(0, 51),
                },
                new()
                {
                    Key = "output_resolution",
                    Control = ControlKind.Select,
                    Options = ResolutionOptions(),
                },
                new()
                {
                    Key = "fps",
                    Control = ControlKind.Select,
                    Options = FrameRateOptions(),
                },
                new()
                {
                    Key = "gop",
                    Control = ControlKind.Select,
                    Options = KeyframeOptions(),
                },
            ],
        },
        new()
        {
            Key = "transport",
            Fields =
            [
                new()
                {
                    Key = "transport",
                    Control = ControlKind.Radio,
                    Options =
                    [
                        new() { Value = "srt" },
                        new() { Value = "rtsp" },
                        new() { Value = "webrtc" },
                        new() { Value = "rtmp" },
                    ],
                },
                new()
                {
                    Key = "srt_publish_latency_ms",
                    Control = ControlKind.Slider,
                    Unit = Unit.Milliseconds,
                    Range = Bounded(20, 2000, 10),
                },
                new()
                {
                    Key = "rtsp_publish_protocol",
                    Control = ControlKind.Select,
                    Options =
                    [
                        new() { Value = "tcp" },
                        new() { Value = "udp" },
                    ],
                },
            ],
        },
        new()
        {
            Key = "network",
            Fields =
            [
                new()
                {
                    Key = "uplink_mbps",
                    Control = ControlKind.Number,
                    Unit = Unit.MegabitsPerSecond,
                    Range = Bounded(1, 10000),
                },
            ],
        },
        new()
        {
            Key = "destination",
            Fields =
            [
                new() { Key = "name", Control = ControlKind.Text },
                new() { Key = "relay_host", Control = ControlKind.Text },
                new() { Key = "relay_port", Control = ControlKind.Number, Range = Bounded(1, 65535) },
                new() { Key = "rtsp_port", Control = ControlKind.Number, Range = Bounded(1, 65535) },
                new() { Key = "webrtc_port", Control = ControlKind.Number, Range = Bounded(1, 65535) },
                new() { Key = "rtmp_port", Control = ControlKind.Number, Range = Bounded(1, 65535) },
                new() { Key = "hls_port", Control = ControlKind.Number, Range = Bounded(1, 65535) },
                new() { Key = "moq_port", Control = ControlKind.Number, Range = Bounded(1, 65535) },
                new() { Key = "api_port", Control = ControlKind.Number, Range = Bounded(1, 65535) },
            ],
        },
    ];
}
