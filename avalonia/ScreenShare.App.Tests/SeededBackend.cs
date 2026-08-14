using System.Runtime.CompilerServices;
using Google.Protobuf.Reflection;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Tests;

/// <summary>
/// A <see cref="Form"/> to render, with nothing behind it, so the setup flow's behaviour is stated in a test
/// rather than in a screenshot.
///
/// It is in the test project because it is the one file that names a codec.
/// A greying written in the app would be a rule written twice, which is what <c>docs/ipc-api.md</c> forbids;
/// in a test it says what a form looks like rather than what the domain is.
///
/// Everything below is a seed and not a rule.
/// The values and refusals are taken from the Go tables they are computed from, <c>capabilities.Codecs</c>,
/// <c>gpupath.Paths</c>, the transport registry and the platform gates, so a test reads against the product's
/// own vocabulary.
/// Those tables are not evaluated here: the greyings are the few that demonstrate each of the four treatments
/// in <c>docs/field-availability.md</c>.
///
/// Nothing above it learns a codec name, a transport name or a label.
/// They cross as data.
///
/// Every read answers from memory with a completed task.
/// Blocking or handing back a null task would leave the flow above unexercised on the timing the gRPC client
/// really gives it.
/// </summary>
internal sealed class SeededBackend : IBackend
{
    /// <summary>Never raised: no probe lands behind a dictionary and nothing else here moves.</summary>
    public event Action? Changed
    {
        add { }
        remove { }
    }

    /// <summary>One option of a seeded select or radio, before the draft picks one.</summary>
    private sealed record OptionSeed
    {
        public required string Value { get; init; }

        /// <summary>What the entry was derived from. null where it needs no annotation.</summary>
        public Text? Note { get; init; }

        /// <summary>Why this combination rules the option out. null where it stands.</summary>
        public Text? Reason { get; init; }
    }

    /// <summary>One control, before the draft supplies its value.</summary>
    private sealed record FieldSeed
    {
        /// <summary><see cref="Settings"/> field this control edits: "publish.codec".</summary>
        public required string Key { get; init; }

        public required ControlKind Control { get; init; }

        /// <summary>What the number means. Unspecified where the control is not a quantity.</summary>
        public Unit Unit { get; init; } = Unit.Unspecified;

        public IReadOnlyList<OptionSeed> Options { get; init; } = [];

        public NumericRange? Range { get; init; }
    }

    /// <summary>One seeded built-in preset, from <c>backend/internal/form/presets.go</c>.</summary>
    private sealed record PresetSeed
    {
        public required string Key { get; init; }

        /// <summary>Pixel format the promise rests on, which is what can make it unreachable.</summary>
        public required string Chroma { get; init; }

        /// <summary>Fields every candidate carries, written onto a copy of the draft.</summary>
        public required Action<PublishSettings> Base { get; init; }

        /// <summary>
        /// Whether settings deliver the promise, which is what the selection is derived from.
        /// A predicate rather than an equality check, because a field the promise says nothing about may move
        /// without leaving it.
        /// </summary>
        public required Func<PublishSettings, bool> Delivers { get; init; }
    }

    /// <summary>Built-in presets, in the order the backend offers them.</summary>
    private static readonly IReadOnlyList<PresetSeed> PresetSeeds =
    [
        new()
        {
            Key = "lossless",
            Chroma = "gbrp",
            Base = publish =>
            {
                publish.Mode = "lossless";
                publish.ColorRange = "pc";
                publish.Fps = 60;
                publish.Bframes = 0;
                publish.Effort = "p7";
                publish.SrtPublishLatencyMs = 60;
            },
            Delivers = publish => publish.Mode == "lossless"
                && publish.Chroma is "gbrp" or "yuv444p"
                && publish.ColorRange == "pc",
        },
        new()
        {
            Key = "gaming",
            Chroma = "yuv420p",
            Base = publish =>
            {
                publish.Mode = "cbr";
                publish.Fps = 60;
                publish.Bframes = 0;
                publish.Effort = "p5";
                publish.BitrateMbps = 40;
                publish.VbvMs = 100;
                publish.SrtPublishLatencyMs = 100;
            },
            Delivers = publish => publish.Mode is "cbr" or "vbr" or "abr" or "crf"
                && publish.Fps >= 60
                && publish.Bframes <= 0
                && publish.SrtPublishLatencyMs <= 250,
        },
        new()
        {
            Key = "readability",
            Chroma = "yuv444p",
            Base = publish =>
            {
                publish.Mode = "crf";
                publish.Fps = 30;
                publish.Bframes = 2;
                publish.Effort = "p7";
                publish.Cq = 18;
                publish.SrtPublishLatencyMs = 300;
            },
            Delivers = publish => publish.Mode == "crf" && publish.Fps <= 30,
        },
    ];

    /// <summary>A run of fields under one heading, in render order.</summary>
    private sealed record GroupSeed
    {
        public required string Key { get; init; }

        public required IReadOnlyList<FieldSeed> Fields { get; init; }

        /// <summary>
        /// Whether a write to these fields is the setting itself rather than a proposal a commit applies
        /// (form.proto, FieldGroup.applied).
        /// Per group, as the real form states it, since a fixture leaving every group staged would never
        /// exercise the applied write path.
        /// </summary>
        public bool Applied { get; init; }
    }

    /// <summary>
    /// Publish engine each capture backend runs, from <c>publish.captureBackends</c>.
    /// Most greyings on this screen hang off it.
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

    /// <summary>Operating system each capture backend needs. The refusal crosses as a code, not as prose.</summary>
    private static readonly IReadOnlyDictionary<string, string> PlatformOf =
        new Dictionary<string, string>
        {
            ["ddagrab"] = "windows",
            ["gdigrab"] = "windows",
            ["d3d11screencapturesrc"] = "windows",
            ["x11grab"] = "linux",
            ["ximagesrc"] = "linux",
            ["kmsgrab"] = "linux",
            ["portal"] = "linux",
            ["avfoundation"] = "darwin",
            ["avfvideosrc"] = "darwin",
        };

    /// <summary>Encoder family behind each codec, for the greyings keyed on family.</summary>
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

    /// <summary>
    /// Codecs whose encoder has an effort ladder, and the steps each one takes.
    ///
    /// Keyed by codec and not by family, because the steps are the encoder's own identifiers: two codecs of
    /// one family can offer different ones, and a codec offering none says nothing about the rest of it.
    /// A codec with no ladder read off it is absent here, which is what greys the control.
    /// </summary>
    private static readonly IReadOnlyDictionary<string, IReadOnlyList<string>> LadderOf =
        new Dictionary<string, IReadOnlyList<string>>
        {
            ["hevc_nvenc"] = ["p7", "p6", "p5", "p4", "p3", "p2", "p1"],
            ["h264_nvenc"] = ["p7", "p6", "p5", "p4", "p3", "p2", "p1"],
            ["av1_nvenc"] = ["p7", "p6", "p5", "p4", "p3", "p2", "p1"],
            ["libx264"] = ["placebo", "veryslow", "slower", "slow", "medium", "fast", "faster", "veryfast", "superfast", "ultrafast"],
            ["libx265"] = ["placebo", "veryslow", "slower", "slow", "medium", "fast", "faster", "veryfast", "superfast", "ultrafast"],
            ["libsvtav1"] = ["0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13"],
        };

    private readonly string _os;

    public SeededBackend(string operatingSystem) => _os = operatingSystem;

    /// <summary>
    /// Why this fixture's machine cannot show what a monitor holds. null where it can.
    ///
    /// Settable, because it is the one catalog fact deciding whether a whole surface is drawn, and a fixture
    /// answering it one way could test one of the two screens.
    /// </summary>
    public Text? NoMonitorPreview { get; init; }

    /// <summary>
    /// The reference set the copy composes names from: a codec named by format and family, and screens with
    /// a height.
    /// Nothing here decides what is legal.
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
        // Two outputs, because one hides every bug a picker can have: a single tile looks the same whether
        // rows are keyed by index or by position, and one screen leaves nothing to pick between.
        catalog.Monitors.Add(new global::ScreenShare.Api.V1.Monitor { Index = 0, Width = 2560, Height = 1440, RefreshHz = 144, Primary = true });
        catalog.Monitors.Add(new global::ScreenShare.Api.V1.Monitor { Index = 1, Width = 1920, Height = 1080, RefreshHz = 60 });

        // The legs the relay serves a player page for, as the backend's tables answer them.
        // Neither is a leg a player opens by address.
        catalog.BrowserWatchTransports.Add(BrowserLegs);
        catalog.NoMonitorPreview = NoMonitorPreview;
        return Task.FromResult(catalog);
    }

    /// <summary>
    /// The catalog's browser legs, in the backend's own order, so a test asserts against the list rather than
    /// restating it.
    /// </summary>
    public static readonly string[] BrowserLegs = ["hls", "webrtc"];

    public Task<Settings> SettingsAsync(CancellationToken cancellation = default)
    {
        // Honoured rather than ignored, so an abandoned read takes the same path whichever implementation is
        // behind the seam.
        return cancellation.IsCancellationRequested
            ? Task.FromCanceled<Settings>(cancellation)
            : Task.FromResult(Defaults());
    }

    /// <summary>
    /// The settings a first start opens on, from the Go <c>settings.Defaults</c> including its per-platform
    /// capture backend.
    /// </summary>
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
            Effort = "p7",
            Capture = _os == "windows" ? "ddagrab" : _os == "darwin" ? "avfoundation" : "x11grab",
            // A fresh installation carries no audio source, so the stream has no second track.
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
    /// Resolves one draft into the screen, from memory.
    /// Repairs nothing, so <see cref="Form.RepairedFieldKeys"/> stays empty: walking a draft to a legal value
    /// would need tables this fixture does not have.
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
    // No pipeline, no relay and no child process stand behind these, so each answers the honest version of
    // that rather than a plausible figure.
    //
    // One stand-in and not several, because IBackend is one seam.
    // A partial implementation would break the compile of every test that touches a new method, for a reason
    // unrelated to what it tests.

    /// <summary>Nothing publishes: the absent <c>Live</c> is what says so.</summary>
    public Task<PublishState> PublishStateAsync(CancellationToken cancellation = default)
        => Task.FromResult(new PublishState());

    /// <summary>
    /// The relay snapshot, unreachable with a reason by default.
    /// An unreachable relay is a snapshot and never a failure, so a screen renders the sentence rather than
    /// an error.
    /// A test needing paths to watch states them.
    /// </summary>
    public RelayStatus Relay { get; set; } = new()
    {
        Reachable = false,
        Error = "no relay behind this shell yet",
    };

    public Task<RelayStatus> RelayStatusAsync(CancellationToken cancellation = default)
        => Task.FromResult(Relay);

    /// <summary>
    /// Players open here, by the pair the contract keys one by.
    /// Empty until a test that needs a viewer already running states one.
    /// </summary>
    public List<WatchKey> Watching { get; } = [];

    public Task<IReadOnlyList<WatchKey>> WatchingAsync(CancellationToken cancellation = default)
        => Task.FromResult<IReadOnlyList<WatchKey>>(Watching.ToList());

    public Task StartPublishAsync(Settings settings, CancellationToken cancellation = default)
    {
        Assert.NotNull(settings, "starting a publish names the settings the encoder runs on");

        Started.Add(settings);
        return Task.CompletedTask;
    }

    /// <summary>Settings handed to StartPublish, oldest first.</summary>
    public List<Settings> Started { get; } = [];

    /// <summary>
    /// Why a save is refused. Empty while saves are accepted.
    /// A test sets it to see what the screen does with the backend's own sentence.
    /// </summary>
    public string SaveRefusal { get; set; } = "";

    public Task ApplyToStreamAsync(Settings settings, CancellationToken cancellation = default)
    {
        Assert.NotNull(settings, "applying to the running stream names the settings it restarts on");

        return Task.CompletedTask;
    }

    /// <summary>
    /// A save that stores nothing.
    /// No read here answers from what was saved, so a test asserts the call rather than the stored value.
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

    /// <summary>Settings handed to SaveSettings, oldest first.</summary>
    public List<Settings> Saved { get; } = [];

    public Task StopPublishAsync(CancellationToken cancellation = default) => Task.CompletedTask;

    // --- The preset store ---------------------------------------------------------
    //
    // The one state this fixture really holds, rather than answering from a fixed list.
    // No event announces a preset, so a card that saved one reads the store again to see it, and a store that
    // never moved could not exercise that.
    // Seeding goes through the same call the screen makes.

    private readonly List<Preset> _presets = [];

    /// <summary>
    /// The notice every read carries. Absent while the store reads cleanly.
    /// A test sets it to separate a list empty because nothing readable remained from one empty because
    /// nothing was saved.
    /// </summary>
    public Text? PresetNotice { get; set; }

    /// <summary>
    /// Why every preset call is refused. Empty while they are accepted.
    /// One switch for read, save and delete, since a screen does the same with the sentence whichever call
    /// produced it.
    /// </summary>
    public string PresetRefusal { get; set; } = "";

    public Task<PresetStore> PresetsAsync(CancellationToken cancellation = default)
        => PresetRefusal.Length > 0
            ? Task.FromException<PresetStore>(new BackendUnavailableException(PresetRefusal))
            : Task.FromResult(new PresetStore([.. _presets], PresetNotice));

    public Task SavePresetAsync(string name, PublishSettings settings, CancellationToken cancellation = default)
    {
        Assert.That(name.Length > 0, "a preset is saved under a name");
        Assert.NotNull(settings, "a preset is the way of publishing it was saved from");

        if (PresetRefusal.Length > 0)
        {
            return Task.FromException(new BackendUnavailableException(PresetRefusal));
        }

        // The name is the identity, so a second save under one replaces rather than appends.
        // Saving over a preset is how one is edited.
        var kept = new Preset { Name = name, Settings = settings.Clone() };
        var at = _presets.FindIndex(preset => preset.Name == name);
        if (at >= 0)
        {
            _presets[at] = kept;
        }
        else
        {
            _presets.Add(kept);
        }

        return Task.CompletedTask;
    }

    public Task DeletePresetAsync(string name, CancellationToken cancellation = default)
    {
        Assert.That(name.Length > 0, "a preset is deleted by the name it was saved under");

        if (PresetRefusal.Length > 0)
        {
            return Task.FromException(new BackendUnavailableException(PresetRefusal));
        }

        // An unknown name is refused, as the backend refuses it: the answer a window gets when another one
        // deleted the preset first.
        if (_presets.RemoveAll(preset => preset.Name == name) == 0)
        {
            return Task.FromException(new BackendUnavailableException($"no preset named '{name}'"));
        }

        return Task.CompletedTask;
    }

    /// <summary>
    /// A figure nothing measured.
    /// No socket stands behind it, so the figure is fixed and a test can assert against it.
    /// </summary>
    public Task<double> MeasureUplinkAsync(CancellationToken cancellation = default)
        => Task.FromResult(MeasuredUplinkMbps);

    /// <summary>What <see cref="MeasureUplinkAsync"/> answers.</summary>
    public const double MeasuredUplinkMbps = 87;

    /// <summary>
    /// A key nothing drew, fixed so a test can assert the field it landed in.
    /// </summary>
    public Task<(string Key, string Id)> CreateGroupAsync(
        RelaySettings relay, CancellationToken cancellation = default)
        => Task.FromResult((DrawnGroupKey, DrawnGroupId));

    /// <summary>What <see cref="CreateGroupAsync"/> answers.</summary>
    public const string DrawnGroupKey = "Zm9ydHktZm91ci1jaGFyYWN0ZXJzLW9mLWJhc2U2NC1rZXk=";

    public const string DrawnGroupId = "AAAAAAAAAAAAAAAAAAAAAAAAAA";

    public Task StartWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
        => Task.CompletedTask;

    public Task StopWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
        => Task.CompletedTask;

    /// <summary>
    /// Pages this fixture was asked to open, oldest first.
    /// A list and not a set: a page cannot be read back, so a second press has to show as a second entry.
    /// </summary>
    public List<WatchKey> Browsed { get; } = [];

    public Task OpenInBrowserAsync(string streamName, string transport, CancellationToken cancellation = default)
    {
        Browsed.Add(new WatchKey { StreamName = streamName, Transport = transport });
        return Task.CompletedTask;
    }

    /// <summary>
    /// Decodes running here, by the pair the contract keys one by.
    /// Read back through <see cref="ReceivingAsync"/>, so a test asserts what the backend was asked to open
    /// rather than what the shell believed it had opened.
    /// </summary>
    public List<WatchKey> Decoded { get; } = [];

    /// <summary>
    /// What every decode here reports about its colour: the transfer characteristic and the verdict on it.
    ///
    /// Both are seeded rather than one derived from the other, because which curves are HDR is the backend's
    /// table and a copy of it here could disagree with it.
    /// </summary>
    public string Transfer { get; set; } = "";

    public bool Hdr { get; set; }

    /// <summary>
    /// Whether anything here rolls an HDR stream down, and what is missing where nothing does.
    /// A machine's facts, so a test about the greyed row states them.
    /// </summary>
    public bool CanToneMap { get; set; }

    public string ToneMapMissing { get; set; } = "";

    /// <summary>Which open decodes were built with the tone-map rung.</summary>
    private readonly Dictionary<WatchKey, bool> _toneMapped = [];

    /// <summary>
    /// Whether the decodes here carry a sound track.
    /// A stream's fact rather than something derived, so a test about the volume states it.
    /// </summary>
    public bool HasAudio { get; set; }

    /// <summary>
    /// What each decode is playing at. A pair nothing asked about plays unchanged, at (1, false).
    /// Read back through <see cref="ReceivingAsync"/>, so a test asserts what the decode plays at rather than
    /// what the shell last sent.
    /// </summary>
    private readonly Dictionary<WatchKey, (double Volume, bool Muted)> _audio = [];

    public Task<IReadOnlyList<ReceiveStream>> ReceivingAsync(CancellationToken cancellation = default)
        => Task.FromResult<IReadOnlyList<ReceiveStream>>(
            Decoded.Select(key => new ReceiveStream
            {
                Stream = key,
                Live = true,
                Transfer = Transfer,
                Hdr = Hdr,
                // What was built and not what was asked for: a machine with nothing to convert with builds
                // the decode without the rung whatever the call said.
                ToneMap = CanToneMap && _toneMapped.GetValueOrDefault(key),
                CanToneMap = CanToneMap,
                ToneMapMissing = CanToneMap ? "" : ToneMapMissing,
                HasAudio = HasAudio,
                Volume = _audio.GetValueOrDefault(key, (1, false)).Volume,
                Muted = _audio.GetValueOrDefault(key, (1, false)).Muted,
            }).ToList());

    /// <summary>
    /// Opens one decode. A pair already open succeeds without opening a second, the idempotence the contract
    /// states (<c>docs/ipc-api.md</c>).
    ///
    /// Tone mapping is built into the decode, so it is recorded against the pair and a second call naming the
    /// other answer replaces it: a call names the state the decode should be in.
    /// </summary>
    public Task StartReceiveAsync(
        string streamName, string transport, bool toneMap = false, CancellationToken cancellation = default)
    {
        var key = new WatchKey { StreamName = streamName, Transport = transport };
        if (!Decoded.Contains(key))
        {
            Decoded.Add(key);
        }
        _toneMapped[key] = toneMap;

        return Task.CompletedTask;
    }

    public Task StopReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
    {
        Decoded.Remove(new WatchKey { StreamName = streamName, Transport = transport });
        return Task.CompletedTask;
    }

    // No audio branch is loud behind a fixture, and the call still succeeds: it names a state, and a
    // fixture's state is whatever it is told, which keeps a caller's idempotence testable.
    // The level is held against the decode and reported back, since a caller computing its next level from
    // what the decode plays at needs a fixture that answers.
    public Task SetReceiveAudioAsync(
        string streamName, string transport, double volume, bool muted, CancellationToken cancellation = default)
    {
        _audio[new WatchKey { StreamName = streamName, Transport = transport }] = (volume, muted);
        return Task.CompletedTask;
    }

    // No GPU and no pipeline stand behind a fixture, so there is nothing to lend.
    // A fake stream of handles would name GPU memory that does not exist, so these refuse instead.
    public Task<FrameChannel> OpenFramesAsync(string streamName, string transport, CancellationToken cancellation = default)
        => throw new BackendUnavailableException("nothing is decoding");

    public Task<FrameChannel> OpenPreviewFramesAsync(CancellationToken cancellation = default)
        => throw new BackendUnavailableException("nothing is publishing with a local preview");

    /// <summary>
    /// Screens being read, in the order they were first asked for.
    /// A test asserts which screens the backend was asked to read rather than which ones the picker believed
    /// it had opened.
    /// </summary>
    public List<int> Previewed { get; } = [];

    /// <summary>
    /// Every start asked for, repeats included.
    /// <see cref="Previewed"/> answers what is running, this how often it was asked: the difference between a
    /// converge that settles and one that calls on every pass.
    /// </summary>
    public List<int> PreviewStarts { get; } = [];

    /// <summary>
    /// Opens one screen's preview. A screen already being read succeeds without opening a second, the
    /// idempotence the contract states (<c>docs/ipc-api.md</c>).
    /// </summary>
    public Task StartMonitorPreviewAsync(int monitor, CancellationToken cancellation = default)
    {
        PreviewStarts.Add(monitor);
        if (!Previewed.Contains(monitor))
        {
            Previewed.Add(monitor);
        }

        return Task.CompletedTask;
    }

    public Task StopMonitorPreviewAsync(int monitor, CancellationToken cancellation = default)
    {
        Previewed.Remove(monitor);
        return Task.CompletedTask;
    }

    public Task<FrameChannel> OpenMonitorFramesAsync(int monitor, CancellationToken cancellation = default)
        => throw new BackendUnavailableException($"nothing is previewing monitor {monitor}");

    /// <summary>
    /// Read off <see cref="Previewed"/> rather than seeded, so a test asserts what the fixture was asked to
    /// read.
    /// Live stays false, since nothing here produces a frame: the state of a picture asked for and not
    /// arrived.
    /// </summary>
    public Task<IReadOnlyList<PreviewedMonitor>> PreviewedMonitorsAsync(CancellationToken cancellation = default)
        => Task.FromResult<IReadOnlyList<PreviewedMonitor>>(
            Previewed.Select(monitor => new PreviewedMonitor { Monitor = monitor }).ToList());

    public Task OpenLogAsync(string path, CancellationToken cancellation = default) => Task.CompletedTask;

    public Task OpenLogsFolderAsync(CancellationToken cancellation = default) => Task.CompletedTask;

    /// <summary>
    /// An event stream that ends at once.
    /// Nothing here changes on its own, so there is no event to deliver, and the real client reads an ending
    /// stream as the backend going away.
    /// </summary>
    public async IAsyncEnumerable<Event> SubscribeAsync(
        [EnumeratorCancellation] CancellationToken cancellation = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        yield break;
    }

    /// <summary>
    /// A level stream that ends at once: nothing here decodes, and ticking silence forever would invent a
    /// decode.
    /// </summary>
    public async IAsyncEnumerable<AudioLevels> SubscribeAudioLevelsAsync(
        [EnumeratorCancellation] CancellationToken cancellation = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        yield break;
    }

    /// <summary>
    /// The pointer stream, never sent on: the seeded settings publish with the pointer drawn into the frames,
    /// which is the mode that sends no position.
    /// </summary>
    public async IAsyncEnumerable<PointerPosition> SubscribePointerAsync(
        [EnumeratorCancellation] CancellationToken cancellation = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        yield break;
    }

    /// <summary>The whole resolve, with no wire in front of it.</summary>
    private Form Resolve(Settings draft)
    {
        var settings = draft.Clone();
        var form = new Form { Settings = settings, Publishable = true };

        foreach (var group in Groups())
        {
            form.Groups.Add(Resolve(group, settings));
        }

        foreach (var preset in PresetSeeds)
        {
            form.Presets.Add(Resolve(preset, settings));
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
    /// One built-in preset against the draft: what applying it writes, or why nothing here reaches it, and
    /// whether the draft already delivers it.
    ///
    /// A real resolve searches for the encoder, pixel format and capture backend keeping the promise
    /// (<c>backend/internal/form/presets.go</c>).
    /// This states the answer instead, unreachable where the seeded chroma rule refuses the format for the
    /// codec the draft names, because searching would be the preset table written twice.
    /// </summary>
    private BuiltinPreset Resolve(PresetSeed seed, Settings settings)
    {
        var reached = settings.Publish.Clone();
        seed.Base(reached);
        reached.Chroma = seed.Chroma;

        var refusal = OptionRefusal("publish.chroma", seed.Chroma, new Settings { Publish = reached });
        var preset = new BuiltinPreset { Key = seed.Key, Selected = seed.Delivers(settings.Publish) };

        if (refusal is null)
        {
            preset.Settings = reached;
        }
        else
        {
            preset.Reason = Say(
                TextCode.PresetUnreachable,
                Id(TextArgName.Preset, seed.Key),
                Id(TextArgName.Transport, settings.Publish.PublishTransport));
        }

        return preset;
    }

    private FieldGroup Resolve(GroupSeed seed, Settings settings)
    {
        var group = new FieldGroup { Key = seed.Key, Applied = seed.Applied };

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
            Live = LiveHere(seed.Key, settings),
            Value = ValueOf(seed.Key, settings),
            // What a fresh installation holds, read out of the defaults through the reader the value goes
            // through.
            // The real form fills it off the same row that reads the draft (backend/internal/form/form.go).
            DefaultValue = ValueOf(seed.Key, Defaults()),
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
    /// The picked option's value in its string form whatever the settings field's type is, so a select over a
    /// number obeys the same invariant as one over a name.
    /// </summary>
    private static string Picked(FieldValue value) => FieldValues.AsText(value);

    /// <summary>
    /// The field's value, read off the draft through the descriptors rather than a switch.
    /// A key is a settings group and a field in it, which is what makes that possible.
    /// It goes through the shell's own reader, so fixture and screen resolve a key the same way.
    /// </summary>
    private static FieldValue ValueOf(string key, Settings settings)
    {
        // The row a reader grows the audio list by is not in the settings, so it answers the default entry,
        // as the real form does (backend/internal/form/form.go, audioEntry).
        // Reading it off the draft would answer an empty kind, which is not a value the control offers.
        if (key == "publish.audio_sources[0].source" && settings.Publish.AudioSources.Count == 0)
        {
            return new FieldValue { Text = "none" };
        }
        return SettingsDraft.Read(settings, key);
    }

    /// <summary>
    /// The four treatments of <c>docs/field-availability.md</c>, seeded.
    /// Each entry mirrors a rule the Go tables evaluate: a hidden backend knob, a disabled field with the
    /// reason a reader can act on, and a live field with a note.
    /// </summary>
    private (bool Visible, bool Enabled, Text? Reason, Text? Note) Availability(string key, Settings settings)
    {
        switch (key)
        {
            // Hidden: a knob of the kmsgrab scanout path alone, so its help text says nothing to a reader on
            // another capture backend.
            case "publish.drm_map":
                return (settings.Publish.Capture == "kmsgrab", true, null, null);

            // Disabled with a reason: a general encoding concept blocked by this combination.
            // The ladder is the codec's own, so the reason names the codec, which is the fact nearest to
            // hand.
            case "publish.effort":
                if (!LadderOf.ContainsKey(settings.Publish.Codec))
                {
                    return (true, false, Say(TextCode.CodecTakesNoEffortLadder,
                        Id(TextArgName.Codec, settings.Publish.Codec)), null);
                }
                return (true, true, null, null);

            case "publish.audio_codec":
                return settings.Publish.AudioSources.All(a => a.Source is "" or "none")
                    ? (true, false, Say(TextCode.AudioCodecNeedsSource), null)
                    : (true, true, null, null);

            case "publish.srt_publish_latency_ms":
                return (settings.Publish.PublishTransport == "srt", true, null, null);

            case "publish.rtsp_publish_protocol":
                return (settings.Publish.PublishTransport == "rtsp", true, null, null);

            // Disabled from the mode rather than from the codec: only the constant-quality mode aims at a
            // quality, so the modes aiming at a bitrate grey the quantizer and name the mode.
            case "publish.cq":
                return settings.Publish.Mode == "crf"
                    ? (true, true, null, null)
                    : (true, false, Say(TextCode.CqOnlyInConstantQuality), null);

            // Live with a note: the value reaches the encoder and means something the heading does not say.
            case "publish.monitor":
                return (true, true, null, Say(TextCode.MonitorNotEnumerated, Num(TextArgName.Monitor, settings.Publish.Monitor)));

            default:
                return (true, true, null, null);
        }
    }

    /// <summary>
    /// Whether a change reaches the pipeline already publishing, from <c>backend/internal/publish/live.go</c>.
    ///
    /// One control carries it: a bitrate, on the engine whose child holds a control socket, in the modes that
    /// send the encoder a rate at all.
    /// Everything else is the pipeline's shape and costs a relaunch.
    /// </summary>
    private bool LiveHere(string key, Settings settings)
    {
        if (key != "publish.bitrate_mbps")
        {
            return false;
        }
        return EngineOf.GetValueOrDefault(settings.Publish.Capture, "") == "gstreamer"
            && settings.Publish.Mode is "cbr" or "vbr" or "abr";
    }

    /// <summary>
    /// Why one option of one field is ruled out.
    /// One refusal is seeded per kind the tables produce: a platform gate, a pair with no device path, and an
    /// engine that lacks the element.
    /// </summary>
    private Text? OptionRefusal(string key, string value, Settings settings)
    {
        var publish = settings.Publish;
        var engine = EngineOf.GetValueOrDefault(publish.Capture, "");

        switch (key)
        {
            case "publish.capture":
                if (!PlatformOf.TryGetValue(value, out var needs) || needs == _os)
                {
                    return null;
                }
                return Say(TextCode.CaptureWrongOs, Id(TextArgName.Capture, value), Id(TextArgName.Os, needs));

            case "publish.audio_sources[0].source":
                return value == "desktop" && _os != "linux"
                    ? Say(TextCode.AudioSourceUnserved, Id(TextArgName.Audio, value), Id(TextArgName.Os, _os))
                    : null;

            case "publish.capture_memory":
                if (value is not ("gpu" or "gpu-encoder-color"))
                {
                    // Never greyed: auto answers with whichever path the pair has, and every pair has the
                    // system copy.
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
                // The two chroma facts holding for every codec in the list: 4:2:2 is the software H.26x rows'
                // alone, and direct RGB needs an encoder that takes a GBR input.
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
    // A fact crosses as a code and the identifiers it is about, so a stand-in builds the same shape.
    // No sentence is written here: it would be a second answer beside the shell's copy.

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

    /// <summary>The pairs with a device path, from <c>gpupath.Paths</c>.</summary>
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
    /// The capture source the quality ladders are derived from, standing in for the monitor
    /// <c>display.List</c> reports.
    /// The numbers are the mockups' own.
    ///
    /// One record rather than three lists, because the lists are consequences of it: a resolution ladder is
    /// the source scaled by whole steps, and a frame-rate list is bounded by what the panel refreshes at.
    /// Listing them instead would write down an answer that depends on which monitor is selected.
    /// </summary>
    private static readonly (int Width, int Height, double RefreshHz) Source = (2560, 1440, 59.951);

    /// <summary>Standard heights a source is offered scaled down to, largest first.</summary>
    private static readonly int[] ScaleHeights = [2160, 1440, 1080, 900, 720, 540];

    /// <summary>Frame rates offered, before the source's refresh rate narrows them.</summary>
    private static readonly int[] FrameRates = [24, 30, 50, 60, 120];

    /// <summary>Keyframe intervals offered, in seconds, before the frame rate turns them into counts.</summary>
    private static readonly int[] KeyframeSeconds = [1, 2, 4];

    /// <summary>
    /// The resolutions this source scales to: its own size, and each standard height below it at the source's
    /// aspect ratio.
    /// Derived and not listed, so another monitor gives another ladder with nothing here edited.
    ///
    /// A scaled entry carries what it was derived from, so the cost of the choice is on the control rather
    /// than in the step that owns the source.
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
    /// The frame rates, every one of them offered.
    /// Above the source's own refresh the extra frames are repeats, which the form states as a diagnostic
    /// rather than as a refusal, so nothing here is greyed
    /// (docs/field-availability.md, "One option disabled with a reason").
    /// </summary>
    private static IReadOnlyList<OptionSeed> FrameRateOptions() =>
        [.. FrameRates.Select(rate => new OptionSeed
        {
            Value = rate.ToString(),
        })];

    /// <summary>
    /// The keyframe intervals, as the frame counts the settings carry.
    ///
    /// Worked out against the source's refresh rather than the draft's frame rate, because recomputing them
    /// per draft would evaluate a rule instead of standing in for one.
    /// The Go side works them out against the resolved frame rate.
    /// </summary>
    private static IReadOnlyList<OptionSeed> KeyframeOptions()
    {
        var fps = (int)Math.Round(Source.RefreshHz);
        var options = new List<OptionSeed>
        {
            // Auto rather than a duration: the encoder's own rule, which every builder reads as twice the
            // frame rate.
            new() { Value = "0" },
        };

        options.AddRange(KeyframeSeconds.Select(seconds => new OptionSeed
        {
            Value = (seconds * fps).ToString(),
        }));

        return options;
    }

    /// <summary>
    /// The seeded screen, in render order.
    /// The grouping follows the domain, what the source is, what encodes it and where it goes, which is why
    /// it is stated here rather than left to a shell to arrange.
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
                // A select and not a number, as the backend answers: one entry per catalog row, so a screen
                // this machine does not have is a missing entry rather than a number typed past the end of
                // the list (backend/internal/form/options.go, optionMonitors).
                new()
                {
                    Key = "publish.monitor",
                    Control = ControlKind.Select,
                    Options = [new() { Value = "0" }, new() { Value = "1" }],
                },
                // The real form draws one field per audio source plus the row a reader grows the list by
                // (backend/internal/form/fields.go).
                // Only the growing row is seeded, since no draft here records a source.
                new()
                {
                    Key = "publish.audio_sources[0].source",
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
                // The NVENC ladder, because every draft seeded here is on an NVENC codec.
                // The backend offers whichever ladder the selected codec declares (LadderOf); this list is
                // static, as every other option list here is.
                new()
                {
                    Key = "publish.effort",
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
        // How this machine receives: a group of the same form, drawn by the viewer rather than by the wizard
        // (Features/Fields/Model/GroupPlacement.cs).
        // Seeded so the split is testable, since a form with no watch group leaves the filter nothing to
        // filter.
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

                        // One greyed watch leg, so the treatment lands on a leg as well: the player this
                        // engine runs opens no HLS address, and the entry keeps its place carrying the two
                        // that would have worked (docs/field-availability.md, "One option disabled with a
                        // reason").
                        new()
                        {
                            Value = "hls",
                            Reason = Say(
                                TextCode.NoViewerReceivesOver,
                                Id(TextArgName.Engine, "ffmpeg"),
                                Id(TextArgName.Transport, "hls"),
                                Ids(TextArgName.Transports, "srt", "rtsp")),
                        },
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
        // The stream's name is staged like everything else the wizard configures, being part of the pipeline
        // a commit starts.
        new()
        {
            Key = "stream",
            Fields =
            [
                new() { Key = "publish.name", Control = ControlKind.Text },
            ],
        },

        // The relay's address, applied rather than staged.
        // The backend dials this address on its own poll, so a write that waited for a publish would gate the
        // publish on reaching the relay it was about to change (form.proto, FieldGroup.applied).
        new()
        {
            Key = "relay",
            Applied = true,
            Fields =
            [
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
