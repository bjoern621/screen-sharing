using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Viewer.Model;
using ScreenShare.App.Features.Viewer.Tile.Model;
using ScreenShare.App.Mvvm;
using TablerIcons;

namespace ScreenShare.App.Features.Viewer.Tile.ViewModel;

/// <summary>
/// One tile: the stream it draws, the figures over it, and the subscription the control behind it reads.
///
/// Two kinds of figure, from opposite directions.
/// What the pipeline is belongs to the backend, read through <see cref="Session.Receiving"/> on every pass.
/// What this window got and drew arrives through <see cref="Report"/> and is the only state stored here:
/// a backend cannot see that a compositor was too slow to take a frame.
///
/// Decides nothing about the decode.
/// Whether a stream is being received is <c>StartReceive</c>'s answer and whether the publish's local preview
/// is running is the publish's; this draws what the subscription hands it and says why when handed nothing.
///
/// One class for every kind of tile.
/// What differs is what a subscription names (<see cref="TileSource"/>) and where the pipeline's state is read
/// from (<see cref="TilePipeline"/>).
/// A second implementation would be a second answer to what a dropped frame is and where a lent handle goes
/// back.
/// </summary>
public sealed class TileViewModel : Observable, IFrameSource
{
    private readonly TileSource _source;
    private readonly IBackend _backend;
    private readonly Action<Action> _dispatch;

    /// <summary>
    /// What the control last reported, and the only state kept here.
    ///
    /// <see cref="TileReport.Nothing"/> rather than the struct's default: a tile renders as soon as it is
    /// added and its first report lands after that, and the default leaves the notice a render reads null.
    /// </summary>
    private TileReport _report = TileReport.Nothing;

    /// <summary>
    /// How far one volume key press moves the level, as a fraction of the 0..1 range.
    /// Audible in one press, fine enough to land on a level rather than beside it.
    /// </summary>
    private const double VolumeStep = 0.05;

    /// <param name="arrange">
    /// Raises this tile's wish to be arranged differently: focused, popped out, filling a screen.
    /// Which tile is focused and which window a stream is drawn in are facts about the whole grid, so the tile
    /// asks and decides none of it.
    /// </param>
    public TileViewModel(TileSource source, IBackend backend, Action<Action> dispatch, Action<TileIntent> arrange)
    {
        Assert.NotNull(source, "a tile names the decode it draws from");
        Assert.NotNull(backend, "a tile subscribes to the backend's frames");
        Assert.NotNull(dispatch, "a tile needs a UI loop to marshal a report back to");
        Assert.NotNull(arrange, "a tile asks the grid to rearrange it rather than rearranging itself");

        _source = source;
        _backend = backend;
        _dispatch = dispatch;

        ToggleFocus = new DelegateCommand(() => arrange(TileIntent.Focus));
        TogglePopOut = new DelegateCommand(() => arrange(TileIntent.PopOut));
        ToggleFullscreen = new DelegateCommand(() => arrange(TileIntent.Fullscreen));
        LeaveFullscreen = new DelegateCommand(() => arrange(TileIntent.LeaveFullscreen));
        LeavePopOut = new DelegateCommand(() => arrange(TileIntent.LeavePopOut));
        // The panel is composed on the render pass and only while it is up, so turning it on has to reach one.
        // Without the notification it opens empty and fills in whenever the next sample lands.
        ToggleStats = new DelegateCommand(() =>
        {
            ShowStats = !ShowStats;
            Changed?.Invoke();
        });
        ToggleMute = new PendingCommand(() => SendAudioAsync(Volume, !Muted), dispatch, () => HasAudio);
        ToggleToneMap = new PendingCommand(() => SendToneMapAsync(!ToneMapped), dispatch, () => CanToneMap);
        // Through the slider's own setter, so one path sends a volume.
        Louder = new DelegateCommand(() => Volume = Math.Clamp(Volume + VolumeStep, 0, 1), () => HasAudio);
        Quieter = new DelegateCommand(() => Volume = Math.Clamp(Volume - VolumeStep, 0, 1), () => HasAudio);
    }

    public TileSource Source => _source;

    /// <summary>Carried and never parsed.</summary>
    public string Name => _source.Name;

    /// <summary>The leg the decode was opened on, empty for either preview.</summary>
    public string Transport => _source.Transport;

    // --- Outputs ---------------------------------------------------------------------

    private string _title = "";
    private string _notice = "";
    private bool _hasNotice;
    private bool _isFocused;
    private bool _isPoppedOut;
    private bool _isFullscreen;
    private bool _showStats;
    private bool _hasAudio;
    private bool _muted;
    private double _volume = 1;
    private double _level;
    private bool _hasLevel;
    private bool _isHdr;
    private bool _toneMapped;
    private bool _canToneMap;
    private string _colourNote = "";
    private bool _hasColourNote;
    private string _toneMapNote = "";
    private double _aspect = TileLayout.UnknownAspect;

    /// <summary>
    /// Width over height of the arriving frames, <see cref="TileLayout.UnknownAspect"/> until a pool has
    /// announced one.
    ///
    /// The frames' shape rather than the box they are drawn in, which is what makes it an input to the
    /// arrangement rather than a result of it.
    /// It moves when the first pool lands, and again where the source resizes.
    /// </summary>
    public double Aspect { get => _aspect; private set => Set(ref _aspect, value); }

    /// <summary>
    /// Pixel size of the frames being drawn, zero until a pool has announced one.
    ///
    /// The scaler's output rather than the decode's reported size, for the reason <see cref="Aspect"/> is.
    /// Anything placed over the picture needs it: a position in the picture's pixels is not a position in the
    /// card's.
    /// </summary>
    public int PictureWidth => _report.Width;

    public int PictureHeight => _report.Height;

    /// <summary>
    /// Written by the screen that owns the arrangement, never here.
    /// At most one tile carries it, and a tile cannot see what the others are doing.
    /// </summary>
    public bool IsFocused { get => _isFocused; set => Set(ref _isFocused, value); }

    /// <summary>
    /// Whether this stream is drawn in a window of its own.
    /// Written by the screen that owns the arrangement, like <see cref="IsFocused"/>.
    ///
    /// The slot stays in the grid at this stream's shape and draws a plate, so nothing reflows when a stream
    /// pops out or comes back.
    /// The plate holds no subscription and asks for no render size: the popped window is the decode's only
    /// consumer, and a black box costing a full-size texture pool would be paid for twice.
    ///
    /// It says where the picture is, not which of the two a card draws.
    /// Both cards read this one tile, so the host that templates a card states that
    /// (<c>Features/Viewer/Tile/View/TileCard.axaml.cs</c>).
    /// </summary>
    public bool IsPoppedOut { get => _isPoppedOut; set => Set(ref _isPoppedOut, value); }

    /// <summary>
    /// Whether the window drawing this stream fills a screen with it.
    ///
    /// Written by the screen that owns the arrangement: which window a stream is in decides which fullscreen
    /// state answers for it, and a tile knows neither.
    /// Carried here so the menu's fullscreen row can show it in force, which a reader inside a filled screen
    /// cannot see otherwise.
    /// </summary>
    public bool IsFullscreen { get => _isFullscreen; set => Set(ref _isFullscreen, value); }

    /// <summary>
    /// Whether the stats panel is up over this tile.
    ///
    /// Per tile rather than per window: an app-wide switch would paint every tile to answer a question about
    /// one.
    /// Off by default, and it dies with the tile.
    /// </summary>
    public bool ShowStats { get => _showStats; private set => Set(ref _showStats, value); }

    /// <summary>Stream and leg, as the heading prints them.</summary>
    public string Title { get => _title; private set => Set(ref _title, value); }

    /// <summary>
    /// Every stage of the pipeline and the figures read off it, in the order frames pass through.
    ///
    /// Empty while the panel is down, so no tile composes a panel's worth of rows per sample to draw none of
    /// them.
    ///
    /// Converged rather than replaced on each pass, so a row keeps its identity, and its open tooltip, for as
    /// long as the pipeline reports the same figures and only the reading is written
    /// (<see cref="Features.Viewer.Tile.Model.TileStats.Merge"/>).
    /// </summary>
    public ObservableCollection<StatSection> Stats { get; } = [];

    /// <summary>Why the tile is dark, empty while it is drawing.</summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    // --- Audio -----------------------------------------------------------------------
    //
    // Read through the decode's state rather than remembered from what was sent (docs/ipc-api.md).
    // The audio branch belongs to the decode, two windows on one decode share it, and a control trusting its
    // own last write would be the second author of a value it does not own.

    /// <summary>
    /// Whether the decode carries an audio track.
    ///
    /// A different fact from silence, and drawn differently: no track is no meter and a greyed volume, where
    /// silence is an empty meter and a live one.
    /// </summary>
    public bool HasAudio { get => _hasAudio; private set => Set(ref _hasAudio, value); }

    /// <summary>Separate from a volume of zero, so unmuting returns to the chosen level.</summary>
    public bool Muted { get => _muted; private set => Set(ref _muted, value); }

    /// <summary>
    /// Linear gain, 0..1, where one is the stream unchanged.
    ///
    /// The setter sends rather than stores: the value comes back on the decode's state and is written here by
    /// the render pass.
    /// A drag issues one effect per step, each naming a state the backend can already be in, which is why the
    /// effect had to be idempotent before a slider could be pointed at it.
    /// </summary>
    public double Volume
    {
        get => _volume;
        set
        {
            if (Math.Abs(_volume - value) < 0.001)
            {
                return;
            }

            _ = SendAudioAsync(value, Muted);
        }
    }

    /// <summary>
    /// How loud the stream is, as a fraction of the meter's span, and zero where nothing is being metered.
    ///
    /// Measured before the volume element, so a muted tile still shows its stream making noise
    /// (<c>backend/internal/receive/audio.go</c>).
    /// </summary>
    public double Level { get => _level; private set => Set(ref _level, value); }

    /// <summary>Whether a measurement has arrived, which separates an empty meter from no meter.</summary>
    public bool HasLevel { get => _hasLevel; private set => Set(ref _hasLevel, value); }

    // --- Colour ----------------------------------------------------------------------
    //
    // Read through the decode's state for the reason the loudness is.
    // What a stream carries is settled when the decoder negotiates and what the pipeline does about it is
    // what the pipeline was built with, so a tile remembering what it asked for would author both twice.

    /// <summary>
    /// Whether this stream carries more range than a standard display shows.
    ///
    /// The backend's verdict on the transfer characteristic rather than a reading of it here: which curves
    /// carry the range is a table on the side that also refuses to publish one in eight bits.
    /// </summary>
    public bool IsHdr { get => _isHdr; private set => Set(ref _isHdr, value); }

    /// <summary>
    /// Whether the decode is rolling that range down into the one this display shows.
    ///
    /// What ran, never what was asked for: a machine with no element for it builds the decode without one,
    /// and a tick reporting the request would claim a conversion nobody made.
    /// </summary>
    public bool ToneMapped { get => _toneMapped; private set => Set(ref _toneMapped, value); }

    /// <summary>
    /// Whether this machine can roll the range down at all, which is what enables the control.
    /// A machine that cannot keeps the row greyed with <see cref="ToneMapNote"/> naming what is absent, rather
    /// than offering a conversion nothing performs.
    /// </summary>
    public bool CanToneMap { get => _canToneMap; private set => Set(ref _canToneMap, value); }

    /// <summary>
    /// What this tile is drawing in colour terms, empty for a stream whose range this display shows.
    ///
    /// The badge over the picture, and the whole of what tells a reader the choice exists: an HDR stream
    /// drawn as it arrives reads as a bad stream rather than as a setting.
    /// </summary>
    public string ColourNote { get => _colourNote; private set => Set(ref _colourNote, value); }

    /// <summary>
    /// Whether the badge has anything to say, which is what draws it.
    /// Written beside the sentence rather than derived from it, as <see cref="HasNotice"/> is: nothing raises
    /// a change for a computed property.
    /// </summary>
    public bool HasColourNote { get => _hasColourNote; private set => Set(ref _hasColourNote, value); }

    /// <summary>
    /// What the tone-map row says beside itself: what it does on a machine that can, what is absent on one
    /// that cannot.
    ///
    /// A greyed control that says nothing teaches nothing, so the row names what would have to be installed
    /// (<c>docs/field-availability.md</c>).
    /// </summary>
    public string ToneMapNote { get => _toneMapNote; private set => Set(ref _toneMapNote, value); }

    // --- What the menu offers --------------------------------------------------------

    /// <summary>Focuses this tile, or gives up focus when it already has it.</summary>
    public DelegateCommand ToggleFocus { get; }

    /// <summary>Draws this stream in a window of its own, or returns it to the grid.</summary>
    public DelegateCommand TogglePopOut { get; }

    /// <summary>Takes the window holding this tile fullscreen, or brings it back.</summary>
    public DelegateCommand ToggleFullscreen { get; }

    /// <summary>
    /// Gives the window holding this tile back to its grid, and does nothing where it fills no screen.
    ///
    /// One direction rather than a toggle, because it is what Escape means: a key that toggled would take a
    /// windowed stream fullscreen from inside a menu the reader was trying to close.
    /// </summary>
    public DelegateCommand LeaveFullscreen { get; }

    /// <summary>
    /// Draws this stream in the grid, and does nothing where it is already there.
    ///
    /// What a closed pop-out window reports, so one direction rather than a toggle: the pass that closes a
    /// window is acting on a stream already given back, and a toggle raised from there would pop the stream
    /// out again into a second window.
    /// </summary>
    public DelegateCommand LeavePopOut { get; }

    /// <summary>Puts the figures up over this tile, or takes them down. They stay up off the pointer.</summary>
    public DelegateCommand ToggleStats { get; }

    /// <summary>Silences this decode, or unsilences it at the volume that was chosen.</summary>
    public PendingCommand ToggleMute { get; }

    /// <summary>
    /// Plays this decode one step louder, and does nothing at the top of the range.
    ///
    /// Names the level it wants rather than a change, like the slider: the value comes back on the decode's
    /// state, so a press computes its target from what the decode plays at now rather than from what a
    /// previous press asked for.
    /// The menu's volume row is where a reader finds it, so it carries no row of its own
    /// (<c>Features/Viewer/Tile/View/TileKeys.cs</c>).
    /// </summary>
    public DelegateCommand Louder { get; }

    /// <summary>Plays this decode one step quieter, and does nothing once it is silent.</summary>
    public DelegateCommand Quieter { get; }

    /// <summary>
    /// Rolls this stream's range down into the one this display shows, or draws it as it arrives.
    /// Enabled on an HDR stream where this machine has the element, greyed with a reason everywhere else.
    /// </summary>
    public PendingCommand ToggleToneMap { get; }

    // Every menu row names a state and holds still.
    // The glyph and the wording are the markup's; what moves is the flag each row reads back as a tick
    // (docs/design-language.md, "Menus").
    // A row worded for what pressing it would do never says what is true, and a menu is read at rest far more
    // often than it is pressed.

    // --- Lifecycle -------------------------------------------------------------------

    /// <summary>
    /// The one render function.
    /// Reads the decode's state through on every pass and combines it with the report the control last made,
    /// so an unchanged pair fires no binding.
    /// </summary>
    /// <param name="sample">
    /// The last sample of this decode, null where none has arrived or where this tile draws no relay decode.
    /// Separate from <paramref name="pipeline"/> because the two are a state and a measurement: what a decode
    /// is gets announced when it changes, what it is doing is read off the pipeline on a clock.
    /// </param>
    public void Apply(TilePipeline? pipeline, ReceiveStreamStats? sample)
    {
        // Only a relay decode crossed a protocol, so only a relay decode has a leg to name beside the stream.
        Title = _source.IsRelay ? $"{Name} · {Words.Transport(Transport)}" : Name;
        Notice = NoticeFor(pipeline);
        HasNotice = Notice.Length > 0;
        Aspect = AspectOf();

        // Composed only while the panel is up: a sample lands once a second, and every tile in the grid would
        // be building a panel's worth of rows to draw none of them.
        TileStats.Merge(Stats, ShowStats ? TileStats.Of(sample, _report) : []);

        // A pipeline that is gone leaves the controls at their defaults rather than at the last one's values:
        // the next decode of this stream starts unchanged and unmuted, and a slider showing otherwise would
        // describe a pipeline that no longer exists.
        // A preview reports no track, so it lands here as a video-only stream does.
        HasAudio = pipeline?.HasAudio ?? false;
        Set(ref _volume, pipeline?.Volume ?? 1, nameof(Volume));
        Muted = pipeline?.Muted ?? false;
        ToggleMute.Refresh();

        if (!HasAudio)
        {
            // A track that has gone takes the meter with it, and no tick arrives to clear the bar: left as it
            // was, it would go on showing the last measurement of a stream that stopped carrying sound.
            HasLevel = false;
            Level = 0;
        }

        // A pipeline that is gone leaves nothing to say about colour: what a stream carried is a fact about a
        // decode that is no longer running.
        // Only a relay decode is offered the choice, because only a relay decode is opened by the call that
        // carries it.
        IsHdr = pipeline?.Hdr ?? false;
        ToneMapped = pipeline?.ToneMap ?? false;
        CanToneMap = _source.IsRelay && IsHdr && (pipeline?.CanToneMap ?? false);
        ColourNote = ColourNoteFor(pipeline);
        HasColourNote = ColourNote.Length > 0;
        ToneMapNote = ToneMapNoteFor(pipeline, _source.IsRelay);
        ToggleToneMap.Refresh();

        Assert.That(HasNotice == (Notice.Length > 0), "a notice and its sentence agree", Name);
        Assert.That(HasColourNote == (ColourNote.Length > 0), "a colour badge and its sentence agree", Name);
        Assert.That(!CanToneMap || IsHdr, "only a stream that carries the range is offered a way out of it", Name);
        Assert.That(Aspect > 0, "a tile has a positive shape to be arranged at", Name, Aspect);
    }

    /// <summary>
    /// Takes one measurement of this decode's loudness, or none.
    ///
    /// Its own entry point rather than part of <see cref="Apply"/>: levels arrive fifteen times a second on a
    /// stream of their own, and the render pass at that rate would re-read the whole session to move one bar
    /// (<c>Backend/Session.cs</c>, <c>Metered</c>).
    /// Writes the two properties the meter binds and nothing else.
    /// </summary>
    public void Meter(AudioLevel? level)
    {
        if (level is null || !HasAudio)
        {
            HasLevel = false;
            Level = 0;
            return;
        }

        HasLevel = true;
        Level = Fraction(level.PeakDb);
    }

    /// <summary>
    /// Where one dBFS reading sits on the meter, 0..1.
    ///
    /// Full-scale decibels run from zero downwards with no bottom (digital silence is negative infinity), so
    /// a bar needs a floor.
    /// Sixty dB of span is what a meter this size shows a difference across, and quieter reads as silence.
    /// </summary>
    private static double Fraction(double db)
    {
        const double floor = -60;

        if (double.IsNaN(db) || db <= floor)
        {
            return 0;
        }

        return Math.Clamp((db - floor) / -floor, 0, 1);
    }

    private double AspectOf()
        => _report.Width > 0 && _report.Height > 0
            ? (double)_report.Width / _report.Height
            : TileLayout.UnknownAspect;

    /// <summary>
    /// Asks for this stream to be drawn rolled down into the range this display shows, or as it arrives.
    ///
    /// The same call that opened the decode.
    /// Tone mapping is an element of the pipeline rather than a value written to a running one, so the state
    /// the decode should be in is named and the backend rebuilds it: a repeat costs nothing, and the tick
    /// comes back through the decode's own state rather than from what was sent.
    ///
    /// The tile goes dark for as long as one decode takes to open, which is why this is a choice rather than
    /// something done to every HDR stream.
    /// </summary>
    private async Task SendToneMapAsync(bool toneMap)
    {
        // Keyed by the stream and the leg a relay decode is identified by, which neither preview has.
        // Nothing on screen offers the choice on one, so this is the guard for a caller that got there anyway.
        if (!_source.IsRelay)
        {
            return;
        }

        try
        {
            await _backend.StartReceiveAsync(Name, Transport, toneMap).ConfigureAwait(false);
        }
        catch (BackendUnavailableException)
        {
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <summary>
    /// Asks the backend for a loudness, and says nothing about what it became.
    ///
    /// The answer arrives as receive state, like every other effect's, so what the slider draws is a fact the
    /// backend stated rather than the value this shell last sent.
    /// A refusal is swallowed: the one refusal this call has is a decode that is no longer running, and the
    /// tile is already drawing that.
    /// </summary>
    private async Task SendAudioAsync(double volume, bool muted)
    {
        // Keyed by the stream and the leg, which neither preview has.
        // A preview reports no track, so nothing on screen offers a volume: this is the guard for a caller
        // that got one anyway.
        if (!_source.IsRelay)
        {
            return;
        }

        try
        {
            await _backend.SetReceiveAudioAsync(Name, Transport, Math.Clamp(volume, 0, 1), muted).ConfigureAwait(false);
        }
        catch (BackendUnavailableException)
        {
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <inheritdoc />
    public Task<FrameChannel> OpenAsync(CancellationToken cancellation)
        => _source.OpenAsync(_backend, cancellation);

    /// <inheritdoc />
    public void Report(TileReport report)
    {
        // Dispatched rather than assumed to be on the UI loop already: it is the one guarantee that a report
        // and an event-driven pass cannot interleave.
        _dispatch(() =>
        {
            _report = report;
            Changed?.Invoke();
        });
    }

    /// <summary>Raised after a report has moved, so the screen holding this tile re-renders.</summary>
    public event Action? Changed;

    /// <summary>
    /// The badge over an HDR picture, empty for a stream whose range this display shows.
    ///
    /// Names the curve and then what is being done about it, which is what a reader comparing two tiles of
    /// one stream is comparing.
    /// Drawn in both states rather than the untouched one alone: a badge that vanished once the conversion
    /// was on would leave the two indistinguishable.
    /// </summary>
    private static string ColourNoteFor(TilePipeline? pipeline)
    {
        if (pipeline is not { Hdr: true } p)
        {
            return "";
        }

        return p.ToneMap
            ? $"{Words.Transfer(p.Transfer)}, rolled down for this display"
            : $"{Words.Transfer(p.Transfer)}, drawn as it arrives";
    }

    /// <summary>
    /// What the tone-map row says beside itself, empty where the control is live and needs no explaining.
    ///
    /// A row this tile cannot take names what is absent, as every refused option in this app does.
    /// An element this build does not register is something to install and a platform with no route at all is
    /// not, so the two do not read alike.
    /// </summary>
    private static string ToneMapNoteFor(TilePipeline? pipeline, bool isRelay)
    {
        if (pipeline is not { } p || !isRelay)
        {
            return "";
        }
        if (!p.Hdr)
        {
            return "This stream carries the range this display shows.";
        }
        if (p.CanToneMap)
        {
            return "";
        }

        return p.ToneMapMissing.Length > 0
            ? $"This GStreamer registers no {p.ToneMapMissing}, which is what rolls the range down."
            : "Nothing on this platform rolls an HDR stream down.";
    }

    /// <summary>
    /// Why this tile is dark, empty while it draws.
    ///
    /// The states a reader has to tell apart, in the order they happen: nothing is decoding, the pipeline is
    /// up and no frame has left it, or the tile could not draw what it was handed.
    /// The last is the control's own sentence, shown as it stands, because it names a driver or a handle type
    /// that nothing else here knows.
    /// </summary>
    private string NoticeFor(TilePipeline? pipeline)
    {
        if (_report.Notice.Length > 0)
        {
            return _report.Notice;
        }

        if (pipeline is null)
        {
            // In the source's own terms: a stream nobody opened a decode for and a screen nobody is reading
            // are different things to act on.
            return _source.Missing;
        }

        return pipeline.Value.Live ? "" : "Connecting.";
    }

}
