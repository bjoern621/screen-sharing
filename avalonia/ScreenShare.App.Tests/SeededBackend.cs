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
        /// <summary>The <see cref="Settings"/> field this control edits, named as that message names it.</summary>
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

    public Task<Settings> SettingsAsync(CancellationToken cancellation = default)
    {
        // Honoured rather than ignored: a caller that has already abandoned this read
        // gets the answer the socket would have given it, so the cancellation path is
        // the same one whichever implementation is behind the seam.
        return cancellation.IsCancellationRequested
            ? Task.FromCanceled<Settings>(cancellation)
            : Task.FromResult(Defaults());
    }

    private Settings Defaults() => new()
    {
        Relay = new RelaySettings
        {
            Host = "127.0.0.1",
            SrtPort = 8890,
            ApiPort = 9997,
            RtspPort = 8554,
            WebrtcPort = 8889,
            RtmpPort = 1935,
            HlsPort = 8888,
        },
        Publish = new PublishSettings
        {
            Name = Environment.MachineName.Length > 0 ? Environment.MachineName : "me",
            PublishTransport = "srt",
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
            RtspPublishProtocol = "tcp",
            UplinkMbps = 50,
            OutputResolution = "",
        },
        Viewer = new ViewerSettings
        {
            PlayerWatchTransport = "srt",
            TileWatchTransport = "srt",
            RtspWatchProtocol = "tcp",
            SrtWatchLatencyMs = 1200,
            RtspWatchLatencyMs = 200,
            RenderChain = "gl",
        },
    };

    /// <summary>
    /// Resolves one draft into the screen, answered from memory. Repairs nothing, so
    /// <see cref="Form.RepairedFieldKeys"/> is always empty: a stand-in that walked the tables
    /// to a legal value would be walking tables it does not have.
    /// </summary>
    public Task<Form> ResolveFormAsync(Settings draft, CancellationToken cancellation = default)
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

    public Task StartPublishAsync(Settings settings, CancellationToken cancellation = default)
    {
        Assert.NotNull(settings, "starting a publish names the settings the encoder runs on");

        Started.Add(settings);
        return Task.CompletedTask;
    }

    /// <summary>The settings handed to StartPublish, oldest first.</summary>
    public List<Settings> Started { get; } = [];

    /// <summary>
    /// Why a save is refused, empty while it is accepted. A test sets it to see what the screen
    /// does with the backend's own sentence, which is the one worth showing.
    /// </summary>
    public string SaveRefusal { get; set; } = "";

    public Task ApplyToStreamAsync(Settings settings, CancellationToken cancellation = default)
    {
        Assert.NotNull(settings, "applying to the running stream names the settings it restarts on");

        return Task.CompletedTask;
    }

    /// <summary>
    /// A save that keeps nothing. Nothing in this fixture reads the settings back, so what a
    /// test asserts is that the call was made rather than what it stored.
    /// </summary>
    public Task SaveSettingsAsync(Settings settings, CancellationToken cancellation = default)
    {
        Assert.NotNull(settings, "saving the settings names the settings to keep");

        if (SaveRefusal.Length > 0)
        {
            return Task.FromException(new BackendUnavailableException(SaveRefusal));
        }

        Saved.Add(settings);
        return Task.CompletedTask;
    }

    /// <summary>The settings handed to SaveSettings, oldest first.</summary>
    public List<Settings> Saved { get; } = [];

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

    public Task<IReadOnlyList<ReceiveStream>> ReceivingAsync(CancellationToken cancellation = default)
        => Task.FromResult<IReadOnlyList<ReceiveStream>>([]);

    public Task StartReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        => Task.CompletedTask;

    public Task StopReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        => Task.CompletedTask;

    // Nothing is decoding behind a fixture, so there is no audio branch to be loud. The call
    // succeeds rather than refusing: what it asks for is a state, and a fixture's state is
    // whatever it is told, which is what keeps a caller's idempotence testable here.
    public Task SetReceiveAudioAsync(
        string streamName, string transport, double volume, bool muted, CancellationToken cancellation = default)
        => Task.CompletedTask;

    // A fixture has no GPU and no pipeline, so there is nothing to lend and nothing to draw.
    // Refusing is the honest answer: a fake stream of handles would be a fake naming GPU
    // memory that does not exist.
    public Task<FrameChannel> OpenFramesAsync(string streamName, string transport, CancellationToken cancellation = default)
        => throw new BackendUnavailableException("nothing is decoding");

    public Task<FrameChannel> OpenPreviewFramesAsync(CancellationToken cancellation = default)
        => throw new BackendUnavailableException("nothing is publishing with a local preview");

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

    /// <summary>
    /// A level stream that ends at once, for the reason the event stream does: nothing here is
    /// decoding, so there is nothing to meter, and a fixture that ticked silence forever would
    /// be inventing a decode.
    /// </summary>
    public async IAsyncEnumerable<AudioLevels> SubscribeAudioLevelsAsync(
        [EnumeratorCancellation] CancellationToken cancellation = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        yield break;
    }

    /// <summary>The whole of the resolve, with no wire in front of it.</summary>
    private Form Resolve(Settings draft)
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
            Estimate = new Estimate { BitrateMbps = 11.8, RawMbps = 3110.4, HeadroomMbps = settings.Publish.UplinkMbps - 11.8 },
        };

        if (settings.Publish.UplinkMbps < form.Summary.Estimate.BitrateMbps)
        {
            form.Diagnostics.Add(new Diagnostic
            {
                Severity = Severity.Warning,
                FieldKey = "publish.uplink_mbps",
                Text = Say(TextCode.UplinkBelowPrediction,
                    Dec(TextArgName.BitrateMbps, form.Summary.Estimate.BitrateMbps),
                    Num(TextArgName.UplinkMbps, settings.Publish.UplinkMbps)),
            });
        }

        Assert.That(form.Groups.Count == Groups().Count, "a resolved group per seeded group", form.Groups.Count);
        return form;
    }

    /// <summary>
    /// One group with its draft-dependent parts filled in: each field's value, its
    /// visibility and enabled state, and which option is picked.
    /// </summary>
    private FieldGroup Resolve(GroupSeed seed, Settings settings)
    {
        var group = new FieldGroup { Key = seed.Key };

        foreach (var field in seed.Fields)
        {
            group.Fields.Add(Resolve(field, settings));
        }

        return group;
    }

    private Field Resolve(FieldSeed seed, Settings settings)
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
    /// The field's current value, read off the draft through the descriptors rather than a
    /// switch. The key is a settings group and a field in it, which is what makes that
    /// possible and what the contract relies on everywhere else. It goes through the shell's
    /// own reader, so the fixture and the screen resolve a key the same way.
    /// </summary>
    private static FieldValue ValueOf(string key, Settings settings) => SettingsDraft.Read(settings, key);

    /// <summary>
    /// The four treatments, seeded. Each entry here mirrors a rule that lives in the Go
    /// tables today and will be evaluated there: a hidden backend knob, a disabled general
    /// concept with the reason the reader can act on, and a live field with a note.
    /// </summary>
    private (bool Visible, bool Enabled, Text? Reason, Text? Note) Availability(string key, Settings settings)
    {
        switch (key)
        {
            // Hidden: a knob of the kmsgrab scanout path and of nothing else, whose help
            // text would teach a reader on another backend nothing.
            case "publish.drm_map":
                return (settings.Publish.Capture == "kmsgrab", true, null, null);

            // Disabled with a reason: a general encoding concept this combination blocks.
            // Where two facts block it, the reason names the one the reader can act on -
            // the family before the engine, since another codec is nearer to hand.
            case "publish.enc_preset":
                if (FamilyOf.GetValueOrDefault(settings.Publish.Codec, "") != "nvenc")
                {
                    return (true, false, Say(TextCode.PresetOnlyOnFamilies, Ids(TextArgName.Families, "nvenc")), null);
                }
                if (EngineOf.GetValueOrDefault(settings.Publish.Capture, "") == "gstreamer")
                {
                    return (true, false, Say(TextCode.GstNoPresetLadder), null);
                }
                return (true, true, null, null);

            case "publish.audio_codec":
                return settings.Publish.Audio == "none"
                    ? (true, false, Say(TextCode.AudioCodecNeedsSource), null)
                    : (true, true, null, null);

            case "publish.srt_publish_latency_ms":
                return (settings.Publish.PublishTransport == "srt", true, null, null);

            case "publish.rtsp_publish_protocol":
                return (settings.Publish.PublishTransport == "rtsp", true, null, null);

            // Disabled with a reason, from the mode rather than from the codec: only the
            // constant-quality mode aims at a quality, so the modes that aim at a bitrate
            // grey the quantizer and name the mode, which is the fact the reader can act on.
            case "publish.cq":
                return settings.Publish.Mode == "crf"
                    ? (true, true, null, null)
                    : (true, false, Say(TextCode.CqOnlyInConstantQuality), null);

            // Live with a note: the value still reaches the encoder and means something the
            // heading does not describe here.
            case "publish.monitor":
                return (true, true, null, Say(TextCode.MonitorNotEnumerated, Num(TextArgName.Monitor, settings.Publish.Monitor)));

            default:
                return (true, true, null, null);
        }
    }

    /// <summary>
    /// Why one option of one field is ruled out here. The seeded set covers the three
    /// kinds the tables produce: a platform gate, a pair with no device path, and an
    /// engine that lacks the element.
    /// </summary>
    private Text? OptionRefusal(string key, string value, Settings settings)
    {
        var publish = settings.Publish;
        var engine = EngineOf.GetValueOrDefault(publish.Capture, "");

        switch (key)
        {
            case "publish.capture":
                if (!PlatformOf.TryGetValue(value, out var platform) || platform.Os == _os)
                {
                    return null;
                }
                return Say(TextCode.CaptureWrongOs, Id(TextArgName.Capture, value), Id(TextArgName.Os, platform.Os));

            case "publish.audio":
                return value == "desktop" && _os != "linux"
                    ? Say(TextCode.AudioSourceUnserved, Id(TextArgName.Audio, value), Id(TextArgName.Os, _os))
                    : null;

            case "publish.capture_memory":
                if (value is not ("gpu" or "gpu-encoder-color"))
                {
                    // Auto and the system copy are never greyed: auto answers with whichever
                    // path the pair has, and the copy is the path every pair has.
                    return null;
                }
                return HasDevicePath(engine, publish.Capture, FamilyOf.GetValueOrDefault(publish.Codec, ""))
                    ? null
                    : Say(
                        TextCode.PairHasNoDeviceMemory,
                        Id(TextArgName.Capture, publish.Capture),
                        Id(TextArgName.Codec, publish.Codec),
                        Id(TextArgName.Engine, engine));

            case "publish.publish_transport":
                return value == "rtmp" && engine == "gstreamer"
                    ? Say(
                        TextCode.EngineHasNoPublishSink,
                        Id(TextArgName.Capture, publish.Capture),
                        Id(TextArgName.Engine, engine),
                        Id(TextArgName.Transport, value))
                    : null;

            case "publish.codec":
                return engine == "gstreamer" && FamilyOf.GetValueOrDefault(value, "") == "amf"
                    ? Say(TextCode.GapGstAmfcodecWindowsOnly)
                    : null;

            case "publish.chroma":
                // Seeded from the two chroma facts that hold for every codec in the list:
                // 4:2:2 is the software H.26x rows' alone, and direct RGB needs a codec
                // whose encoder takes a GBR input.
                var family = FamilyOf.GetValueOrDefault(publish.Codec, "");
                if (value == "yuv422p" && family != "software")
                {
                    return Say(
                        TextCode.CodecCannotEncodeChroma,
                        Id(TextArgName.Codec, publish.Codec),
                        Id(TextArgName.Chroma, value));
                }
                if (value == "gbrp" && publish.Codec is not ("hevc_nvenc" or "libx265" or "libvpx-vp9"))
                {
                    return Say(TextCode.CodecCodesNoRgb, Id(TextArgName.Codec, publish.Codec));
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
                    Key = "publish.capture",
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
                    Key = "publish.capture_memory",
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
                    Key = "publish.drm_map",
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
                    Key = "publish.monitor",
                    Control = ControlKind.Number,
                    Range = Bounded(0, 7),
                },
                new()
                {
                    Key = "publish.audio",
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
                    Key = "publish.codec",
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
                    Key = "publish.chroma",
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
                    Key = "publish.color_range",
                    Control = ControlKind.Select,
                    Options =
                    [
                        new() { Value = "pc" },
                        new() { Value = "tv" },
                    ],
                },
                new()
                {
                    Key = "publish.enc_preset",
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
                    Key = "publish.audio_codec",
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
                    Key = "publish.mode",
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
                    Key = "publish.cq",
                    Control = ControlKind.Slider,
                    Range = Bounded(0, 51),
                },
                new()
                {
                    Key = "publish.output_resolution",
                    Control = ControlKind.Select,
                    Options = ResolutionOptions(),
                },
                new()
                {
                    Key = "publish.fps",
                    Control = ControlKind.Select,
                    Options = FrameRateOptions(),
                },
                new()
                {
                    Key = "publish.gop",
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
                    Key = "publish.publish_transport",
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
                    Key = "publish.srt_publish_latency_ms",
                    Control = ControlKind.Slider,
                    Unit = Unit.Milliseconds,
                    Range = Bounded(20, 2000, 10),
                },
                new()
                {
                    Key = "publish.rtsp_publish_protocol",
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
                    Key = "publish.uplink_mbps",
                    Control = ControlKind.Number,
                    Unit = Unit.MegabitsPerSecond,
                    Range = Bounded(1, 10000),
                },
            ],
        },
        // How this machine receives, which is a group of the same form and is drawn by the
        // viewer rather than by the wizard (Features/Fields/Model/GroupPlacement.cs). It is in
        // the fixture so the split is testable at all: a form with no watch group would let the
        // filter pass by having nothing to filter.
        new()
        {
            Key = "watch",
            Fields =
            [
                new()
                {
                    Key = "viewer.player_watch_transport",
                    Control = ControlKind.Select,
                    Options =
                    [
                        new() { Value = "srt" },
                        new() { Value = "rtsp" },
                        new() { Value = "hls" },
                    ],
                },
                new()
                {
                    Key = "viewer.tile_watch_transport",
                    Control = ControlKind.Select,
                    Options =
                    [
                        new() { Value = "srt" },
                        new() { Value = "rtsp" },
                        new() { Value = "whep" },
                    ],
                },
                new()
                {
                    Key = "viewer.render_chain",
                    Control = ControlKind.Select,
                    Options =
                    [
                        new() { Value = "gl" },
                        new() { Value = "sys" },
                    ],
                },
                new()
                {
                    Key = "viewer.srt_watch_latency_ms",
                    Control = ControlKind.Slider,
                    Unit = Unit.Milliseconds,
                    Range = Bounded(20, 8000, 10),
                },
            ],
        },
        new()
        {
            Key = "destination",
            Fields =
            [
                new() { Key = "publish.name", Control = ControlKind.Text },
                new() { Key = "relay.host", Control = ControlKind.Text },
                new() { Key = "relay.srt_port", Control = ControlKind.Number, Range = Bounded(1, 65535) },
                new() { Key = "relay.rtsp_port", Control = ControlKind.Number, Range = Bounded(1, 65535) },
                new() { Key = "relay.webrtc_port", Control = ControlKind.Number, Range = Bounded(1, 65535) },
                new() { Key = "relay.rtmp_port", Control = ControlKind.Number, Range = Bounded(1, 65535) },
                new() { Key = "relay.hls_port", Control = ControlKind.Number, Range = Bounded(1, 65535) },
                new() { Key = "relay.api_port", Control = ControlKind.Number, Range = Bounded(1, 65535) },
            ],
        },
    ];
}
