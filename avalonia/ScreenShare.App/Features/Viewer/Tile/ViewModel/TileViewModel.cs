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
/// One tile of the grid: the stream it draws, the figures over it, and the subscription the
/// control behind it reads.
///
/// <b>It holds two kinds of figure and they come from opposite directions.</b> What the
/// pipeline is - the render chain that ran, the memory at each end, the decoder - is the
/// backend's and is read through <see cref="Session.Receiving"/> on every pass, like every
/// other state this shell draws. What this window got and drew is the tile's own and can come
/// from nowhere else: a backend cannot see that a compositor was too slow to take a frame.
/// The second kind arrives through <see cref="Report"/> and is the only state this class
/// stores.
///
/// <b>It decides nothing about the decode.</b> Whether a stream is being received is
/// <c>StartReceive</c>'s answer and the roster's business, and whether the publish's local
/// preview is running is the publish's; this draws whatever the subscription hands it and
/// says why when it is handed nothing.
///
/// <b>It is one class for both kinds of tile, and that is deliberate.</b> The viewer's grid
/// draws relay decodes and the broadcast screen draws the local preview, and the difference
/// between them is entirely in what a subscription names (<see cref="TileSource"/>) and where
/// the pipeline's own state is read from (<see cref="TilePipeline"/>). A second implementation
/// would be a second answer to what a dropped frame is and where a lent handle goes back.
/// </summary>
public sealed class TileViewModel : Observable, IFrameSource
{
    private readonly TileSource _source;
    private readonly IBackend _backend;
    private readonly Action<Action> _dispatch;

    /// <summary>
    /// What the control last reported. The one piece of state here, and it is this window's own.
    ///
    /// It starts at <see cref="TileReport.Nothing"/> rather than at the struct's default,
    /// because a tile is rendered as soon as it is added and its first report arrives after
    /// that: a default carries a null notice, and the render reads the notice.
    /// </summary>
    private TileReport _report = TileReport.Nothing;

    /// <param name="arrange">
    /// Asks the screen holding this tile to arrange it differently: focus it, pop it out, put it
    /// on a screen of its own. The tile raises the intent and decides none of it - which tile is
    /// focused and which window a stream is drawn in are facts about the whole grid, and a tile
    /// that wrote them would be one of several authors of one arrangement.
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
        ToggleStats = new DelegateCommand(() => ShowStats = !ShowStats);
        ToggleMute = new PendingCommand(() => SendAudioAsync(Volume, !Muted), dispatch, () => HasAudio);
    }

    /// <summary>Which decode this tile draws from, read through rather than taken apart.</summary>
    public TileSource Source => _source;

    /// <summary>The stream name, carried and never parsed.</summary>
    public string Name => _source.Name;

    /// <summary>The leg the decode this tile draws was opened on, empty for the local preview.</summary>
    public string Transport => _source.Transport;

    // --- Outputs ---------------------------------------------------------------------

    private string _title = "";
    private string _notice = "";
    private bool _hasNotice;
    private bool _isLive;
    private bool _isFocused;
    private bool _isPoppedOut;
    private bool _isFullscreen;
    private bool _showStats;
    private bool _hasAudio;
    private bool _muted;
    private double _volume = 1;
    private double _level;
    private bool _hasLevel;
    private double _aspect = TileLayout.UnknownAspect;

    /// <summary>
    /// Width over height of the stream this tile draws, and the assumed shape until the frame
    /// channel has announced a pool.
    ///
    /// It is the shape of the frames that arrive rather than of the box they are drawn in, which
    /// is what makes it an input to the arrangement instead of a result of it. It moves once per
    /// stream in the ordinary case - when the first pool lands - and again if the source resizes.
    /// </summary>
    public double Aspect { get => _aspect; private set => Set(ref _aspect, value); }

    /// <summary>
    /// The size of the frames this tile is drawing, and zero until a pool has announced one.
    ///
    /// It is the frames own size rather than the decode's reported one, for the reason
    /// <see cref="Aspect"/> is: what a tile draws is what the scaler produced for it. Anything
    /// placing something over the picture needs it, because a position in the picture's pixels
    /// is not a position in the card's.
    /// </summary>
    public int PictureWidth => _report.Width;

    public int PictureHeight => _report.Height;

    /// <summary>
    /// Whether this is the focused tile. Written by the screen that owns the arrangement, never
    /// here: at most one tile carries it, and a tile cannot know what the others are doing.
    /// </summary>
    public bool IsFocused { get => _isFocused; set => Set(ref _isFocused, value); }

    /// <summary>
    /// Whether this stream is being drawn in a window of its own.
    ///
    /// The tile keeps its place in the grid while it is: the slot stays, at this stream's shape,
    /// and draws a plate saying where the picture went. Nothing reflows when a stream pops out
    /// and nothing reflows when it comes back, which is the point of keeping the slot.
    ///
    /// The plate holds no subscription and asks for no render size. The popped window is the only
    /// consumer, and a black box costing a full-size texture pool would be the one arrangement
    /// this shell pays for twice.
    ///
    /// <b>It says where the picture is, not what a card draws.</b> Both cards read this one tile
    /// while a stream is popped out, so which of them draws the plate is stated by the host that
    /// templates it (<c>Features/Viewer/Tile/View/TileCard.axaml.cs</c>). A card that decided it
    /// from here would draw the plate in the window that was opened to hold the picture.
    /// </summary>
    public bool IsPoppedOut { get => _isPoppedOut; set => Set(ref _isPoppedOut, value); }

    /// <summary>
    /// Whether the window drawing this stream is filling a screen with it.
    ///
    /// Written by the screen that owns the arrangement, like the two above and for the same
    /// reason: which window a stream is in decides which fullscreen state answers for it, and a
    /// tile knows neither. It is here so the menu's fullscreen row can show whether it is in
    /// force, which is the one thing a reader cannot see from inside a filled screen.
    /// </summary>
    public bool IsFullscreen { get => _isFullscreen; set => Set(ref _isFullscreen, value); }

    /// <summary>
    /// Whether the figures are drawn over this tile permanently.
    ///
    /// Per tile rather than per window: it is turned on for the one stream being diagnosed, and
    /// an app-wide switch would paint six tiles to answer a question about one. Off by default,
    /// and it dies with the tile - a diagnostic that outlived the question is a diagnostic
    /// nobody turned off.
    /// </summary>
    public bool ShowStats { get => _showStats; private set => Set(ref _showStats, value); }

    /// <summary>The stream and the leg, as the tile's heading prints them.</summary>
    public string Title { get => _title; private set => Set(ref _title, value); }

    /// <summary>
    /// What the pipeline and this window turned out to be, as the strip under the picture
    /// prints it. A list rather than named slots, so what a tile reports stays the tile's.
    /// </summary>
    public IReadOnlyList<string> Figures { get; private set; } = [];

    /// <summary>Why the tile is dark, empty while it is drawing.</summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    /// <summary>Whether a frame has been drawn, which separates a live tile from a connecting one.</summary>
    public bool IsLive { get => _isLive; private set => Set(ref _isLive, value); }

    // --- Audio -----------------------------------------------------------------------
    //
    // All three are read through the decode's state rather than remembered from what was sent
    // (docs/ipc-api.md): the loudness belongs to the decode, two windows on one decode share one
    // audio branch, and a slider that trusted its own last write would be the second author of a
    // value it does not own.

    /// <summary>
    /// Whether the decode carries an audio track at all.
    ///
    /// False is a different fact from silence and draws differently: no meter and a greyed
    /// volume, rather than an empty meter and a live one.
    /// </summary>
    public bool HasAudio { get => _hasAudio; private set => Set(ref _hasAudio, value); }

    /// <summary>Whether the decode is muted. Separate from a volume of zero, so unmuting returns to the chosen level.</summary>
    public bool Muted { get => _muted; private set => Set(ref _muted, value); }

    /// <summary>
    /// How loud the decode plays, from zero to one.
    ///
    /// The setter is what the slider writes, and it sends rather than stores: the value comes
    /// back on the decode's state and is written here by the render pass. A drag therefore issues
    /// one effect per step, each of which is a state the backend can already be in - which is why
    /// the effect had to be idempotent before a slider could be pointed at it.
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
    /// How loud the stream actually is, as a fraction of the meter's span, and zero where nothing
    /// is being metered.
    ///
    /// It is measured before the volume element, so a muted tile still shows its stream making
    /// noise - which is how a reader notices they muted the one that started talking
    /// (<c>internal/receive/audio.go</c>).
    /// </summary>
    public double Level { get => _level; private set => Set(ref _level, value); }

    /// <summary>Whether a measurement has arrived, which is what separates an empty meter from no meter.</summary>
    public bool HasLevel { get => _hasLevel; private set => Set(ref _hasLevel, value); }

    // --- What the menu offers --------------------------------------------------------

    /// <summary>Focuses this tile, or gives up focus when it already has it.</summary>
    public DelegateCommand ToggleFocus { get; }

    /// <summary>Draws this stream in a window of its own, or returns it to the grid.</summary>
    public DelegateCommand TogglePopOut { get; }

    /// <summary>Takes the window holding this tile fullscreen, or brings it back.</summary>
    public DelegateCommand ToggleFullscreen { get; }

    /// <summary>
    /// Gives the window holding this tile back to its grid, and does nothing when it is not
    /// filling a screen.
    ///
    /// One direction rather than a toggle, because it is what Escape means: a key that toggled
    /// would take a windowed stream fullscreen from inside a menu the reader was trying to close.
    /// </summary>
    public DelegateCommand LeaveFullscreen { get; }

    /// <summary>
    /// Draws this stream in the grid, and does nothing when it is already there.
    ///
    /// What a closed pop-out window reports. One direction rather than a toggle, because a window
    /// that has closed is news about a state rather than a request to change one: the pass that
    /// closes a window is itself acting on a stream that was already given back, and a toggle
    /// raised from there would pop the stream straight out again into a second window.
    /// </summary>
    public DelegateCommand LeavePopOut { get; }

    /// <summary>Draws the figures over this tile permanently, or stops.</summary>
    public DelegateCommand ToggleStats { get; }

    /// <summary>Silences this decode, or unsilences it at the volume that was chosen.</summary>
    public PendingCommand ToggleMute { get; }

    // Every row in the menu names one arrangement and holds still. The glyph and the wording are
    // the markup's, because neither of them moves; what moves is the state each row reports, and
    // that is the five flags above and beside this block, which the row draws as a tick
    // (docs/design-language.md, "Menus").
    //
    // The pair of properties per row that used to be here - a label reading "Unmute" on a muted
    // stream and a glyph swapping under it - said what pressing the row would do and therefore
    // never said what was true. A menu is read at rest far more often than it is pressed.

    // --- Lifecycle -------------------------------------------------------------------

    /// <summary>
    /// The one render function. Reads the decode's state through on every pass and combines it
    /// with the report the control last made, so an unchanged pair fires no binding.
    /// </summary>
    public void Apply(TilePipeline? pipeline)
    {
        // Only a relay decode crossed a protocol, so only a relay decode has one to name
        // beside the stream. Everything else on the heading is the same fact either way.
        Title = _source.IsRelay ? $"{Name} · {Words.Transport(Transport)}" : Name;
        IsLive = _report.Live;

        Notice = NoticeFor(pipeline);
        HasNotice = Notice.Length > 0;
        Figures = FiguresFor(pipeline);
        Aspect = AspectOf();

        // The loudness is the pipeline's and is read through it. A pipeline that is gone leaves
        // the controls at their defaults rather than at whatever the last one was set to: the
        // next decode of this stream starts unchanged and unmuted, and a slider showing
        // otherwise would be describing a pipeline that no longer exists. A preview reports no
        // track at all, so it lands here as a video-only stream does.
        HasAudio = pipeline?.HasAudio ?? false;
        Set(ref _volume, pipeline?.Volume ?? 1, nameof(Volume));
        Muted = pipeline?.Muted ?? false;
        ToggleMute.Refresh();

        if (!HasAudio)
        {
            // No track is no meter. Left where it was, a bar would go on showing the last
            // measurement of a stream that has stopped carrying sound.
            HasLevel = false;
            Level = 0;
        }

        Assert.That(HasNotice == (Notice.Length > 0), "a notice and its sentence agree", Name);
        Assert.That(Aspect > 0, "a tile has a positive shape to be arranged at", Name, Aspect);
    }

    /// <summary>
    /// Takes one measurement of this decode's loudness, or none.
    ///
    /// <b>Its own entry point, and not part of <see cref="Apply"/>.</b> Levels arrive fifteen
    /// times a second on a stream of their own, and running the render pass at that rate would
    /// re-read the whole session to move one bar (<c>Backend/Session.cs</c>, <c>Metered</c>).
    /// This writes the two properties the meter binds and nothing else.
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
    /// Where one decibel reading sits on the meter, from zero to one.
    ///
    /// Decibels relative to full scale run from zero downwards with no bottom - digital silence
    /// is negative infinity - so a bar needs a floor, and this is it. Sixty decibels of span is
    /// the range a meter of this size can show a difference across; quieter than that reads as
    /// silence, which for a bar a few pixels tall it is.
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

    /// <summary>
    /// The shape the tile is arranged at: the frames' own once a pool has announced one, and the
    /// assumed shape until then.
    ///
    /// The frames' size rather than the decode's reported one, because what a tile draws is what
    /// the scaler produced for it and the two differ by exactly the bound this tile asked for.
    /// </summary>
    private double AspectOf()
        => _report.Width > 0 && _report.Height > 0
            ? (double)_report.Width / _report.Height
            : TileLayout.UnknownAspect;

    /// <summary>
    /// Asks the backend for a loudness, and says nothing about what it became.
    ///
    /// The answer arrives as receive state, like every other effect's does, so what the slider
    /// draws is a fact the backend stated rather than the value this shell last sent. A refusal
    /// is swallowed here on purpose: the one refusal this call has is a decode that is no longer
    /// running, and the tile is already drawing that.
    /// </summary>
    private async Task SendAudioAsync(double volume, bool muted)
    {
        // The effect is keyed by the pair a relay decode is identified by, and neither preview
        // has either half of it. Refusing here rather than at the control is the honest shape:
        // a preview reports no track, so nothing on screen offers a volume in the first place,
        // and this is the guard for a caller that got one anyway.
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
        // The control is on the UI loop already, but the dispatch is kept rather than assumed:
        // it is the one guarantee that a report and an event-driven pass cannot interleave.
        _dispatch(() =>
        {
            _report = report;
            Changed?.Invoke();
        });
    }

    /// <summary>Raised after a report has moved, so the screen holding this tile re-renders.</summary>
    public event Action? Changed;

    /// <summary>
    /// Why this tile is dark.
    ///
    /// Three states a reader has to be able to tell apart, and the order is the order they
    /// happen in: nothing is decoding this pair, the pipeline is up and no frame has left it,
    /// or the tile itself could not draw what it was handed. The last one is the control's own
    /// sentence and is shown as it stands, because it names a driver or a handle type and
    /// nothing else here knows either.
    /// </summary>
    private string NoticeFor(TilePipeline? pipeline)
    {
        if (_report.Notice.Length > 0)
        {
            return _report.Notice;
        }

        if (pipeline is null)
        {
            // In the source's own terms: a stream nobody opened a decode for and a screen
            // nobody is reading are different things to do something about.
            return _source.Missing;
        }

        return pipeline.Value.Live ? "" : "Connecting.";
    }

    /// <summary>
    /// What the strip prints: what the pipeline turned out to be, then what this window did
    /// with it.
    ///
    /// The memory pair is the figure worth having and the reason the backend reports both
    /// ends. A chain that promised to keep the frames on the device and a decoder that
    /// downloaded its own output disagree, and the pair is the only place that shows.
    /// </summary>
    private IReadOnlyList<string> FiguresFor(TilePipeline? state)
    {
        if (state is not { } decode)
        {
            return [];
        }

        var figures = new List<string>(5);

        if (_report.Width > 0)
        {
            figures.Add($"{_report.Width}×{_report.Height}");
        }

        if (decode.Decoder.Length > 0)
        {
            figures.Add(decode.Hardware ? $"{decode.Decoder} on the GPU" : decode.Decoder);
        }

        if (decode.Chain.Length > 0)
        {
            figures.Add($"{decode.Chain} chain");
        }

        if (decode.RenderMemory.Length > 0)
        {
            figures.Add(MemoryLabel(decode.RenderMemory));
        }

        if (_report.Dropped > 0)
        {
            figures.Add($"{_report.Dropped} dropped");
        }

        return figures;
    }

    /// <summary>
    /// How a memory feature reads on screen. The identifiers are GStreamer's own and cross the
    /// contract unchanged; what they are called here is this shell's, like every other word.
    /// </summary>
    private static string MemoryLabel(string memory) => memory switch
    {
        "memory:SystemMemory" => "frames in system memory",
        "memory:D3D11Memory" => "frames on the GPU, Direct3D 11",
        "memory:D3D12Memory" => "frames on the GPU, Direct3D 12",
        "memory:GLMemory" => "frames on the GPU, OpenGL",
        "memory:DMABuf" => "frames on the GPU, dmabuf",
        _ => memory,
    };
}
