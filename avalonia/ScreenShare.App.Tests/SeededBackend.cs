using System.Runtime.CompilerServices;
using Google.Protobuf.Reflection;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Tests;

/// <summary>
/// A <see cref="Form"/> to render, without a backend behind it.
/// This is the fixture the seam exists for: the setup flow can be driven by something other than a running
/// service, so its behaviour is stated in a test rather than in a screenshot.
///
/// <b>It lives in the test project, and that placement is the point.</b> It was the shipped shell's stand-in
/// until <see cref="ControlBackend"/> answered over the local socket, and it is the one file that names a
/// codec: a greying written here is a rule written twice, which is exactly what <c>docs/ipc-api.md</c> exists
/// to prevent.
/// In the app that would be a defect.
/// In a test it is a fixture, and it says what a form looks like rather than what the domain is.
///
/// Everything here is therefore a seed rather than a rule.
/// The values, labels and refusal sentences are taken from the Go tables they are really computed from -
/// <c>capabilities.Codecs</c>, <c>gpupath.Paths</c>, the transport registry and the platform gates - so a
/// test reads against the product's own vocabulary.
/// What it does not do is evaluate those tables: the greyings below are the few that demonstrate each of the
/// four treatments in <c>docs/field-availability.md</c>, not the full evaluation.
///
/// The one structural rule it keeps is the contract's own: nothing above it learns a codec name, a transport
/// name or a label.
/// They cross as data.
///
/// It answers both reads from memory, so both hand back an already-completed task.
/// What it must not do is block or hand back a null task, because the flow above it is written against the
/// gRPC client's timing and would then never be exercised on the path it really runs.
/// </summary>
internal sealed class SeededBackend : IBackend
{
    /// <summary>
    /// Never raised.
    /// A fixture answering from a dictionary has no probe landing behind it and nothing else that moves, so
    /// the accessors are empty rather than backed by a field nothing would ever invoke.
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

    /// <summary>
    /// One seeded built-in preset: what it writes, the pixel format it asks for, and how to tell whether
    /// settings already deliver it.
    /// Seeded from <c>internal/form/presets.go</c>.
    /// </summary>
    private sealed record PresetSeed
    {
        public required string Key { get; init; }

        /// <summary>The pixel format the promise rests on, which is what can make it unreachable.</summary>
        public required string Chroma { get; init; }

        /// <summary>The fields every candidate carries, written onto a copy of the draft.</summary>
        public required Action<PublishSettings> Base { get; init; }

        /// <summary>
        /// Whether settings deliver the promise, which is the claim the real backend derives the selection
        /// from.
        /// It is a predicate rather than an equality check because that is the difference between the two
        /// kinds of preset: a field the promise says nothing about may move without leaving it.
        /// </summary>
        public required Func<PublishSettings, bool> Delivers { get; init; }
    }

    /// <summary>The three built-in presets, in the order the backend offers them.</summary>
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

    /// <summary>One seeded group: a run of fields under a heading, in render order.</summary>
    private sealed record GroupSeed
    {
        public required string Key { get; init; }

        public required IReadOnlyList<FieldSeed> Fields { get; init; }

        /// <summary>
        /// Whether a write to this group's fields is the setting itself rather than a proposal a commit
        /// applies (form.proto, FieldGroup.applied).
        /// It is seeded per group because the real form states it per group, and a fixture that left every
        /// group staged would let the write path pass by never exercising it.
        /// </summary>
        public bool Applied { get; init; }
    }

    /// <summary>
    /// The publish engine each capture backend runs, which is the fact most greyings on this screen hang off.
    /// Seeded from <c>publish.captureBackends</c>.
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

    /// <summary>
    /// The codecs whose encoder has an effort ladder, and the steps each one takes.
    ///
    /// It is keyed by codec rather than by family because the ladder is the encoder's own: the steps are its
    /// identifiers, so two codecs of one family can offer different ones and a codec that offers none says
    /// nothing about the rest of its family.
    /// The hardware rows the backend has not read a ladder off yet are absent, which is what greys the
    /// control for them.
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
    /// Why this fixture's machine cannot show what a monitor holds, and null where it can.
    ///
    /// Settable, because it is the one catalog fact that decides whether a whole surface is drawn: the
    /// wizard's screen pictures are offered where a session can read one output apart from another and
    /// nowhere else, and a fixture that could only answer one way could only test one of the two screens.
    /// </summary>
    public Text? NoMonitorPreview { get; init; }

    /// <summary>
    /// The settings a first start opens on, answered from memory.
    /// Seeded from the Go <c>settings.Defaults</c>, including its per-platform capture backend.
    /// </summary>
    /// <summary>
    /// The reference set, answered from memory with the two rows a name is composed from: one codec so a
    /// dropdown entry can be named by its format and family, and one screen so the picture shorthand has a
    /// height.
    /// The fixture states what the copy reads, and nothing here decides what is legal.
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
        // Two outputs, because one is the shape that hides every bug a screen picker can have: a grid with
        // one tile in it looks the same whether the picker keyed its rows by index or by position, and a
        // machine with one screen has nothing to pick between.
        catalog.Monitors.Add(new global::ScreenShare.Api.V1.Monitor { Index = 0, Width = 2560, Height = 1440, RefreshHz = 144, Primary = true });
        catalog.Monitors.Add(new global::ScreenShare.Api.V1.Monitor { Index = 1, Width = 1920, Height = 1080, RefreshHz = 60 });

        // The legs the relay serves a player page for, as the backend's own tables answer them: the two the
        // browser reaches, and neither of them a leg a player opens by address.
        catalog.BrowserWatchTransports.Add(BrowserLegs);
        catalog.NoMonitorPreview = NoMonitorPreview;
        return Task.FromResult(catalog);
    }

    /// <summary>
    /// The browser legs the catalog above names, in the backend's own order, so a test asserts against the
    /// list rather than restating it.
    /// </summary>
    public static readonly string[] BrowserLegs = ["hls", "webrtc"];

    public Task<Settings> SettingsAsync(CancellationToken cancellation = default)
    {
        // Honoured rather than ignored: a caller that has already abandoned this read gets the answer the
        // socket would have given it, so the cancellation path is the same one whichever implementation is
        // behind the seam.
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
            Effort = "p7",
            Capture = _os == "windows" ? "ddagrab" : _os == "darwin" ? "avfoundation" : "x11grab",
            // No source: the list a fresh installation carries is empty, which is a stream with no second
            // track.
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
    /// Resolves one draft into the screen, answered from memory.
    /// Repairs nothing, so <see cref="Form.RepairedFieldKeys"/> is always empty: a stand-in that walked the
    /// tables to a legal value would be walking tables it does not have.
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
    // This fixture exists for the setup flow, which is the surface with the cross-field rules worth stating
    // in a test.
    // The running state is not seeded: there is no pipeline behind this, no relay and no child process, so
    // each of these answers the honest version of that rather than a plausible-looking figure.
    //
    // They are here rather than on a second stand-in because IBackend is one seam.
    // A partial implementation would make every test that touches a new method fail to compile for a reason
    // that has nothing to do with what it is testing - which is exactly what happened to this file once
    // already.

    /// <summary>Nothing publishes. The absent <c>Live</c> is what says so.</summary>
    public Task<PublishState> PublishStateAsync(CancellationToken cancellation = default)
        => Task.FromResult(new PublishState());

    /// <summary>
    /// What the relay snapshot says.
    /// No relay answered by default, carrying the reason: an unreachable relay is a snapshot and never a
    /// failure, so a screen driven by this fixture renders the sentence rather than an error.
    /// A test that needs paths to watch states them.
    /// </summary>
    public RelayStatus Relay { get; set; } = new()
    {
        Reachable = false,
        Error = "no relay behind this shell yet",
    };

    public Task<RelayStatus> RelayStatusAsync(CancellationToken cancellation = default)
        => Task.FromResult(Relay);

    /// <summary>
    /// The players this fixture has open, by the pair the contract keys one by.
    /// Empty by default, because nothing has been asked for; a test that needs a viewer already running
    /// states it.
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

    /// <summary>The settings handed to StartPublish, oldest first.</summary>
    public List<Settings> Started { get; } = [];

    /// <summary>
    /// Why a save is refused, empty while it is accepted.
    /// A test sets it to see what the screen does with the backend's own sentence, which is the one worth
    /// showing.
    /// </summary>
    public string SaveRefusal { get; set; } = "";

    public Task ApplyToStreamAsync(Settings settings, CancellationToken cancellation = default)
    {
        Assert.NotNull(settings, "applying to the running stream names the settings it restarts on");

        return Task.CompletedTask;
    }

    /// <summary>
    /// A save that keeps nothing.
    /// Nothing in this fixture reads the settings back, so what a test asserts is that the call was made
    /// rather than what it stored.
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

    // --- The preset store ---------------------------------------------------------
    //
    // Kept in memory rather than answered from a fixed list, and it is the one state this fixture really
    // holds.
    // Presets are the state on this seam that no event announces, so a card that saved one has to read the
    // store again to see it - and a store that never moved could not exercise that at all.
    // What is seeded is done through the same call the screen makes, so a test states the store by writing to
    // it.

    private readonly List<Preset> _presets = [];

    /// <summary>
    /// The notice every read carries, absent while the store reads cleanly.
    /// A test sets it to see what a screen says about a list that is empty because nothing readable remained,
    /// rather than because nothing was saved.
    /// </summary>
    public Text? PresetNotice { get; set; }

    /// <summary>
    /// Why every preset call is refused, empty while they are accepted.
    /// One switch for all three, because what a screen does with the backend's sentence is the same whichever
    /// call produced it.
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

        // The name is the identity, so a second save under one replaces rather than appends - which is what
        // makes saving over a preset the way one is edited.
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

        // A name the store does not hold is refused, as the backend refuses it: that is the answer a window
        // gets when another one deleted the preset first.
        if (_presets.RemoveAll(preset => preset.Name == name) == 0)
        {
            return Task.FromException(new BackendUnavailableException($"no preset named '{name}'"));
        }

        return Task.CompletedTask;
    }

    /// <summary>
    /// A measurement nothing measured.
    /// There is no socket behind this fixture, so it answers a fixed figure a test can assert against rather
    /// than a plausible-looking random one.
    /// </summary>
    public Task<double> MeasureUplinkAsync(CancellationToken cancellation = default)
        => Task.FromResult(MeasuredUplinkMbps);

    /// <summary>What <see cref="MeasureUplinkAsync"/> answers, so a test can name it.</summary>
    public const double MeasuredUplinkMbps = 87;

    public Task StartWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
        => Task.CompletedTask;

    public Task StopWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
        => Task.CompletedTask;

    /// <summary>
    /// The pages this fixture was asked to open, in the order they were asked for.
    /// It is a list and not a set, because a page cannot be read back: what a test can assert about it is the
    /// call, so a second press has to be visible as a second entry.
    /// </summary>
    public List<WatchKey> Browsed { get; } = [];

    public Task OpenInBrowserAsync(string streamName, string transport, CancellationToken cancellation = default)
    {
        Browsed.Add(new WatchKey { StreamName = streamName, Transport = transport });
        return Task.CompletedTask;
    }

    /// <summary>
    /// The decodes this fixture is running, by the pair the contract keys one by.
    /// It is written by the two calls below and read back by <see cref="ReceivingAsync"/>, so a test asserts
    /// what the backend was asked to open rather than what the shell believed it had opened.
    /// </summary>
    public List<WatchKey> Decoded { get; } = [];

    /// <summary>
    /// What every decode this fixture runs reports about its colour: the transfer characteristic the stream
    /// carries and the verdict on it.
    ///
    /// Both are seeded rather than one read off the other, because that is how they arrive: which curves are
    /// HDR is the backend's table, and a fixture deriving the verdict here would be a second copy of it that
    /// could disagree.
    /// </summary>
    public string Transfer { get; set; } = "";

    public bool Hdr { get; set; }

    /// <summary>
    /// Whether this fixture has anything to roll an HDR stream down with, and what is absent where it has
    /// not.
    /// They are a machine's facts, so a test that is about the greyed row states them rather than the fixture
    /// inventing either.
    /// </summary>
    public bool CanToneMap { get; set; }

    public string ToneMapMissing { get; set; } = "";

    /// <summary>Which open decodes were built with the rung, by the pair they are keyed by.</summary>
    private readonly Dictionary<WatchKey, bool> _toneMapped = [];

    /// <summary>
    /// Whether the decodes this fixture runs carry a sound track.
    /// A machine's fact rather than something derived here, so a test about the volume states it.
    /// </summary>
    public bool HasAudio { get; set; }

    /// <summary>
    /// What each decode is playing at, and the unchanged level for a pair nothing has asked about.
    /// It is written by <see cref="SetReceiveAudioAsync"/> and read back through
    /// <see cref="ReceivingAsync"/>, so a test asserts what the decode plays at rather than what the shell
    /// last sent.
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
                // What was built and not what was asked for, which is the backend's own fallback: a machine
                // with nothing to convert with builds the decode without the rung whatever the call said.
                ToneMap = CanToneMap && _toneMapped.GetValueOrDefault(key),
                CanToneMap = CanToneMap,
                ToneMapMissing = CanToneMap ? "" : ToneMapMissing,
                HasAudio = HasAudio,
                Volume = _audio.GetValueOrDefault(key, (1, false)).Volume,
                Muted = _audio.GetValueOrDefault(key, (1, false)).Muted,
            }).ToList());

    /// <summary>
    /// Opens one decode, and answers a pair that is already open without opening a second - the idempotence
    /// the contract states, so a caller that repeats a start is testable against this fixture
    /// (<c>docs/ipc-api.md</c>).
    ///
    /// Tone mapping is part of what the decode is built from, so it is recorded against the pair and a second
    /// call naming the other answer replaces it.
    /// That is the contract's own shape: the call names the state the decode should be in.
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

    // Nothing is decoding behind a fixture, so there is no audio branch to be loud.
    // The call succeeds rather than refusing: what it asks for is a state, and a fixture's state is whatever
    // it is told, which is what keeps a caller's idempotence testable here.
    // The pair is held against the decode and reported back, because a caller that computes its next level
    // from what the decode plays at can only be tested against a fixture that answers.
    public Task SetReceiveAudioAsync(
        string streamName, string transport, double volume, bool muted, CancellationToken cancellation = default)
    {
        _audio[new WatchKey { StreamName = streamName, Transport = transport }] = (volume, muted);
        return Task.CompletedTask;
    }

    // A fixture has no GPU and no pipeline, so there is nothing to lend and nothing to draw.
    // Refusing is the honest answer: a fake stream of handles would be a fake naming GPU memory that does not
    // exist.
    public Task<FrameChannel> OpenFramesAsync(string streamName, string transport, CancellationToken cancellation = default)
        => throw new BackendUnavailableException("nothing is decoding");

    public Task<FrameChannel> OpenPreviewFramesAsync(CancellationToken cancellation = default)
        => throw new BackendUnavailableException("nothing is publishing with a local preview");

    /// <summary>
    /// The screens this fixture is reading, in the order they were first asked for.
    /// It is written by the two calls below, so a test asserts which screens the backend was asked to read
    /// rather than which ones the picker believed it had opened.
    /// </summary>
    public List<int> Previewed { get; } = [];

    /// <summary>
    /// Every start this fixture was asked for, repeats included.
    /// <see cref="Previewed"/> answers what is running and this answers how often it was asked - which is the
    /// difference between a converge that settles and one that calls on every pass.
    /// </summary>
    public List<int> PreviewStarts { get; } = [];

    /// <summary>
    /// Opens one screen's preview, and answers a screen already being read without opening a second - the
    /// idempotence the contract states, so a caller that repeats a start is testable against this fixture
    /// (<c>docs/ipc-api.md</c>).
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
    /// Read back off <see cref="Previewed"/> rather than seeded, so a test asserts what the fixture was asked
    /// to read.
    /// Live is false throughout: nothing behind this fixture produces a frame, which is the state a picture
    /// that has been asked for and not arrived is in.
    /// </summary>
    public Task<IReadOnlyList<PreviewedMonitor>> PreviewedMonitorsAsync(CancellationToken cancellation = default)
        => Task.FromResult<IReadOnlyList<PreviewedMonitor>>(
            Previewed.Select(monitor => new PreviewedMonitor { Monitor = monitor }).ToList());

    public Task OpenLogAsync(string path, CancellationToken cancellation = default) => Task.CompletedTask;

    public Task OpenLogsFolderAsync(CancellationToken cancellation = default) => Task.CompletedTask;

    /// <summary>
    /// A stream that ends at once.
    /// Nothing behind this fixture changes on its own, so there is no event to deliver; ending is what the
    /// real client reads as the backend going away, and it is preferable to a stream that never yields and
    /// never returns.
    /// </summary>
    public async IAsyncEnumerable<Event> SubscribeAsync(
        [EnumeratorCancellation] CancellationToken cancellation = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        yield break;
    }

    /// <summary>
    /// A level stream that ends at once, for the reason the event stream does: nothing here is decoding, so
    /// there is nothing to meter, and a fixture that ticked silence forever would be inventing a decode.
    /// </summary>
    public async IAsyncEnumerable<AudioLevels> SubscribeAudioLevelsAsync(
        [EnumeratorCancellation] CancellationToken cancellation = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        yield break;
    }

    /// <summary>
    /// The pointer stream, which this fixture never sends on: the seeded settings publish with the pointer
    /// drawn into the frames, which is the mode that sends no position.
    /// </summary>
    public async IAsyncEnumerable<PointerPosition> SubscribePointerAsync(
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
    /// One built-in preset against the draft: what applying it would write here, or why nothing here reaches
    /// it, and whether the draft already delivers it.
    ///
    /// The real backend searches for the encoder, pixel format and capture backend that keep the promise
    /// (<c>internal/form/presets.go</c>).
    /// This states the answer instead: the preset writes its own fields, asks for its pixel format, and is
    /// unreachable where this fixture's own chroma rule refuses that format for the codec the draft names.
    /// A stand-in that searched would be the preset table written twice, which is the thing the fixture is
    /// careful not to be.
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

    /// <summary>
    /// One group with its draft-dependent parts filled in: each field's value, its visibility and enabled
    /// state, and which option is picked.
    /// </summary>
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
            // What a fresh installation would hold here, read out of this fixture's own defaults through the
            // same reader the value goes through.
            // The real form fills it the same way, off the row that reads the draft (internal/form/form.go).
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
    /// The value the picked option carries, for the assertion above.
    /// It is the option's own string form whatever the settings field's type is, so a select over a number is
    /// held to the same invariant as one over a name.
    /// </summary>
    private static string Picked(FieldValue value) => FieldValues.AsText(value);

    /// <summary>
    /// The field's current value, read off the draft through the descriptors rather than a switch.
    /// The key is a settings group and a field in it, which is what makes that possible and what the contract
    /// relies on everywhere else.
    /// It goes through the shell's own reader, so the fixture and the screen resolve a key the same way.
    /// </summary>
    private static FieldValue ValueOf(string key, Settings settings)
    {
        // The row a reader grows the audio list by is not in the settings, so it holds the default entry: no
        // kind, at unity.
        // The real form answers it the same way (internal/form/form.go, audioEntry), and reading it off the
        // draft instead would hand the fixture an entry with an empty kind, which is not a value the control
        // offers.
        if (key == "publish.audio_sources[0].source" && settings.Publish.AudioSources.Count == 0)
        {
            return new FieldValue { Text = "none" };
        }
        return SettingsDraft.Read(settings, key);
    }

    /// <summary>
    /// The four treatments, seeded.
    /// Each entry here mirrors a rule that lives in the Go tables today and will be evaluated there: a hidden
    /// backend knob, a disabled general concept with the reason the reader can act on, and a live field with
    /// a note.
    /// </summary>
    private (bool Visible, bool Enabled, Text? Reason, Text? Note) Availability(string key, Settings settings)
    {
        switch (key)
        {
            // Hidden: a knob of the kmsgrab scanout path and of nothing else, whose help text would teach a
            // reader on another backend nothing.
            case "publish.drm_map":
                return (settings.Publish.Capture == "kmsgrab", true, null, null);

            // Disabled with a reason: a general encoding concept this combination blocks.
            // The ladder is the codec's own, so a codec whose encoder has no such knob greys naming itself;
            // where the engine is the second fact blocking it, the reason names the codec first, since
            // another codec is nearer to hand.
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

            // Disabled with a reason, from the mode rather than from the codec: only the constant-quality
            // mode aims at a quality, so the modes that aim at a bitrate grey the quantizer and name the
            // mode, which is the fact the reader can act on.
            case "publish.cq":
                return settings.Publish.Mode == "crf"
                    ? (true, true, null, null)
                    : (true, false, Say(TextCode.CqOnlyInConstantQuality), null);

            // Live with a note: the value still reaches the encoder and means something the heading does not
            // describe here.
            case "publish.monitor":
                return (true, true, null, Say(TextCode.MonitorNotEnumerated, Num(TextArgName.Monitor, settings.Publish.Monitor)));

            default:
                return (true, true, null, null);
        }
    }

    /// <summary>
    /// Whether a change to one control reaches the pipeline that is already publishing, seeded from
    /// <c>internal/publish/live.go</c>.
    ///
    /// One control carries it: the encoder takes a new bitrate while it runs, on the engine whose child holds
    /// a control socket, in the modes that send the encoder a rate at all.
    /// Everything else is part of the pipeline's shape and costs a relaunch.
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
    /// Why one option of one field is ruled out here.
    /// The seeded set covers the three kinds the tables produce: a platform gate, a pair with no device path,
    /// and an engine that lacks the element.
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

            case "publish.audio_sources[0].source":
                return value == "desktop" && _os != "linux"
                    ? Say(TextCode.AudioSourceUnserved, Id(TextArgName.Audio, value), Id(TextArgName.Os, _os))
                    : null;

            case "publish.capture_memory":
                if (value is not ("gpu" or "gpu-encoder-color"))
                {
                    // Auto and the system copy are never greyed: auto answers with whichever path the pair
                    // has, and the copy is the path every pair has.
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
                // Seeded from the two chroma facts that hold for every codec in the list: 4:2:2 is the
                // software H.26x rows' alone, and direct RGB needs a codec whose encoder takes a GBR input.
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
    // The backend states a fact as a code and the identifiers it is about, so a fixture standing in for one
    // builds the same shape.
    // These four are what that takes, and they are the whole of the fixture's vocabulary: no sentence is
    // written here, because a sentence written here would be a second answer beside the shell's own.

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
    /// The capture source the three quality ladders are derived from, standing in for the monitor
    /// <c>display.List</c> reports.
    /// Seeded from the mockups, and the numbers are the mockups' own.
    ///
    /// It is one record rather than three lists because the lists are consequences of it: a resolution ladder
    /// is the source scaled by whole steps and a frame-rate list is bounded by what the panel refreshes at.
    /// Writing the lists out instead would be writing down an answer that depends on which monitor is
    /// selected.
    /// </summary>
    private static readonly (int Width, int Height, double RefreshHz) Source = (2560, 1440, 59.951);

    /// <summary>The standard heights a source is offered scaled down to, largest first.</summary>
    private static readonly int[] ScaleHeights = [2160, 1440, 1080, 900, 720, 540];

    /// <summary>The frame rates offered, before the source's refresh rate narrows them.</summary>
    private static readonly int[] FrameRates = [24, 30, 50, 60, 120];

    /// <summary>The keyframe intervals offered, in seconds, before the frame rate turns them into counts.</summary>
    private static readonly int[] KeyframeSeconds = [1, 2, 4];

    /// <summary>
    /// The resolutions this source can be scaled to: its own size, and each standard height below it at the
    /// source's aspect ratio.
    /// Derived rather than listed, so another monitor produces another ladder with nothing here edited.
    ///
    /// Every entry carries what it was derived from, which is what makes the control honest: the reader sees
    /// the cost of the choice without opening the step that owns the source.
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
    /// A neighbouring source allows them, so the entry stays and says which side the limit is on
    /// (docs/field-availability.md, "One option disabled with a reason").
    /// </summary>
    private static IReadOnlyList<OptionSeed> FrameRateOptions() =>
        [.. FrameRates.Select(rate => new OptionSeed
        {
            // Above the source's own refresh rate the extra frames are repeats, which the form states as a
            // diagnostic rather than as a refusal - so the entry stays offered here, exactly as the tables
            // leave it.
            Value = rate.ToString(),
        })];

    /// <summary>
    /// The keyframe intervals.
    /// The settings carry a frame count, so the label is the concept and the note is the number it works out
    /// to: a reader picks two seconds and can still see it became 120 frames.
    ///
    /// Seeded against the default frame rate rather than the draft's, because a seed that recomputed the
    /// counts per draft would be evaluating a rule instead of standing in for one.
    /// The Go side derives them from the resolved frame rate.
    /// </summary>
    private static IReadOnlyList<OptionSeed> KeyframeOptions()
    {
        var fps = (int)Math.Round(Source.RefreshHz);
        var options = new List<OptionSeed>
        {
            // Auto is not a duration: it is the encoder's own rule, which every builder reads as twice the
            // frame rate.
            // It carries what that works out to, so the entry is measured in the same unit as the ones below
            // it.
            new() { Value = "0" },
        };

        options.AddRange(KeyframeSeconds.Select(seconds => new OptionSeed
        {
            Value = (seconds * fps).ToString(),
        }));

        return options;
    }

    /// <summary>
    /// The seeded screen.
    /// Order is render order, and the grouping follows the domain - what the source is, what encodes it,
    /// where it goes - which is why it is stated here rather than left to a shell to arrange.
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
                // A select and not a number, which is what the backend answers with: the entries are the
                // enumerated outputs, one per catalog row, so a screen this machine does not have is an entry
                // that is not there rather than a number typed past the end of the list
                // (internal/form/options.go, optionMonitors).
                new()
                {
                    Key = "publish.monitor",
                    Control = ControlKind.Select,
                    Options = [new() { Value = "0" }, new() { Value = "1" }],
                },
                // One entry of the audio source list, which the real form draws once per entry plus once for
                // the row a reader grows the list by.
                // The fixture seeds the growing row alone, because that is the one every draft here has: none
                // of them records anything (internal/form/fields.go).
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
                // The NVENC ladder, because every draft this fixture seeds is on an NVENC codec.
                // The backend offers whichever ladder the selected codec declares (LadderOf), and the seeds
                // here are static, as every other option list in this fixture is.
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
        // How this machine receives, which is a group of the same form and is drawn by the viewer rather than
        // by the wizard (Features/Fields/Model/GroupPlacement.cs).
        // It is in the fixture so the split is testable at all: a form with no watch group would let the
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

                        // One greyed watch leg, so the treatment every option gets is on a leg as well: the
                        // player this fixture's engine runs opens no HLS address, and the entry keeps its
                        // place carrying the two that would have worked (docs/field-availability.md, "One
                        // option disabled with a reason").
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
        // The stream's own name is staged like everything else the wizard configures: it is part of the
        // pipeline a commit starts.
        new()
        {
            Key = "stream",
            Fields =
            [
                new() { Key = "publish.name", Control = ControlKind.Text },
            ],
        },

        // Where the relay is, applied rather than staged.
        // The backend dials this address on its own poll, so a write to it that waited for a publish would be
        // a publish gated on reaching the relay it was about to change (form.proto, FieldGroup.applied).
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
