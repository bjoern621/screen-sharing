using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Features.Viewer.Tile.Model;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.Preview.ViewModel;

/// <summary>
/// The program preview: what viewers see, and what it costs to send it.
///
/// <b>What it draws is this machine's own stream, pulled back off the relay.</b> The preview
/// opens a decode with <c>StartReceive</c> and subscribes to its frames exactly as the viewer
/// screen does for anyone else's stream, so the picture on this card has been encoded, sent,
/// re-served and decoded - which is the question the broadcast screen exists to answer.
///
/// <b>The route is a loopback rather than a tee, and the reason is where the encoder runs.</b>
/// Publishing is an external <c>gst-launch-1.0</c> or <c>ffmpeg</c> child process
/// (<c>internal/publish/gstreamer.go</c>, <c>internal/publish/supervise.go</c>), which is what
/// keeps an encoder that dies from taking the backend with it. Splitting the encoded stream
/// in-process would mean giving that crash isolation up, or inventing a second private
/// transport with its own port, its own payloader and a synthetic stream identity for a
/// picture nobody outside this window would ever see. The loopback adds no concept at all:
/// <c>WatchKey{this stream, this leg}</c> is a true statement about a decode, there stays
/// exactly one frame path in the repository, and what the card shows is what a viewer gets,
/// degradation included. What it costs is the relay round trip and the downstream bandwidth,
/// and the card says so in its own words rather than pretending to be a local mirror
/// (<see cref="Cost"/>).
///
/// <b>It holds no reading of its own and no decode list of its own.</b> The stream name comes
/// from the running publish state and what is decoding comes from
/// <see cref="Backend.Session.Receiving"/>, both read through on every pass. The leg is the
/// settings' answer, read through <see cref="TileLeg"/> - the same site the viewer's grid
/// reads, so neither screen picks a protocol.
///
/// <b>The lifecycle is a converge and not a sequence.</b> <see cref="Apply"/> states the world
/// it wants - a decode open for this stream on this leg and a tile subscribed to it, or
/// nothing open - and <see cref="Converge"/> decides what has to be called. A second pass over
/// unchanged input calls nothing, opens nothing and restarts nothing.
///
/// <b>It never calls <c>StopReceive</c>, and that is deliberate.</b> A decode is keyed by the
/// stream and the leg and by nothing else, and the backend does not count its consumers: a
/// stop tears the pipeline down and ends every subscription on it
/// (<c>internal/app/receive.go</c>, <c>internal/receive/receiver.go</c>). The reader can be
/// watching their own stream on the viewer screen at the same time, over the same leg - the
/// tile leg is one setting - so a stop from here would close their tile. Which of two
/// consumers may close a shared decode is not a question this shell may answer, so it answers
/// none of it: the preview drops its subscription and leaves the decode running, and the
/// decode ends when the stream it is receiving does. That the bandwidth goes on being spent
/// until then is stated on the card.
/// </summary>
public sealed class PreviewViewModel : Observable
{
    private readonly IBackend _backend;
    private readonly Session _session;
    private readonly Action<Action> _dispatch;

    /// <summary>
    /// The leg a tile's decode is opened on, empty until the settings have been resolved once.
    /// It is the settings' value and never a choice made here.
    /// </summary>
    private string _leg = "";

    /// <summary>Whether the leg has been asked for, so a failed read is retried and a good one is not repeated.</summary>
    private bool _askedLeg;

    /// <summary>
    /// Whether this card is in a visual tree. It is the one fact the view knows and the view
    /// model cannot read: the shell renders every destination on every pass, so without it a
    /// reader who never opened this screen would still be paying for a decode of their own
    /// stream. Written by <see cref="SetShowing"/> and by nothing else.
    /// </summary>
    private bool _showing;

    /// <summary>
    /// The decode a start has been asked for, and null while none is wanted.
    ///
    /// <b>It is a guard against asking twice for one want, not a memory of what happened.</b>
    /// What is decoding is read back off the session; this only stops the pass that runs
    /// between the request and its answer from sending the request again. It is cleared the
    /// moment the wanted decode changes - which includes going off air and going off screen -
    /// so a want that comes back is asked for again.
    /// </summary>
    private WatchKey? _asked;

    /// <summary>
    /// The tile this card draws, made once the start it belongs to has answered and dropped
    /// when the wanted decode moves off it.
    ///
    /// It is <see cref="TileViewModel"/> and not a second frame consumer written here. A tile
    /// is a stream, the figures over it and a subscription the control behind it reads, and
    /// this card is one of those - a second implementation would be a second answer to what a
    /// dropped frame is and where a handle goes back.
    /// </summary>
    private TileViewModel? _tile;

    /// <summary>The backend's own sentence when it refused the start, empty otherwise.</summary>
    private string _refusal = "";

    /// <param name="dispatch">
    /// Hands work to the UI loop. The answer to the start and the tile's own reports both land
    /// on whichever thread the transport completed on, and everything this writes is read by a
    /// binding that only tolerates being written from one.
    /// </param>
    public PreviewViewModel(IBackend backend, Session session, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a preview asks the backend to open the decode it draws");
        Assert.NotNull(session, "a preview renders the session's running state");
        Assert.NotNull(dispatch, "a preview needs a UI loop to marshal an answer back to");

        _backend = backend;
        _session = session;
        _dispatch = dispatch;

        Apply();
    }

    // --- Inputs -------------------------------------------------------------------

    private BroadcastSnapshot _snapshot = BroadcastSnapshot.Unread;

    public BroadcastSnapshot Snapshot
    {
        get => _snapshot;
        set
        {
            Assert.NotNull(value, "a preview renders a reading");

            if (Set(ref _snapshot, value))
            {
                Apply();
            }
        }
    }

    /// <summary>
    /// Says whether the card is on screen. The named write of that state, and idempotent:
    /// telling it what it already holds re-renders and converges to the same world.
    ///
    /// The view calls it, because whether a control is in a visual tree is a fact only the
    /// control can see. Nothing else writes it, and no render pass reads a toolkit to find it
    /// out for itself.
    /// </summary>
    public void SetShowing(bool showing)
    {
        _showing = showing;
        Apply();
    }

    // --- Outputs ------------------------------------------------------------------

    private string _placeholder = "";
    private bool _hasPlaceholder;
    private string _cost = "";
    private string _encoded = "";
    private string _quality = "";
    private bool _isOnAir;
    private bool _hasTile;

    /// <summary>
    /// The tile the picture is drawn from, and null while there is none. The control bound to
    /// it opens the frame subscription when it is attached and cancels it when it is not, so
    /// clearing this is what ends the subscription (<c>Features/Viewer/Tile/View/StreamTile.cs</c>).
    ///
    /// It reads the same field <see cref="Draw"/> writes, rather than a second copy of it:
    /// one tile, one owner.
    /// </summary>
    public TileViewModel? Tile { get => _tile; private set => Set(ref _tile, value); }

    /// <summary>Whether a tile is on the card, which is what separates it from its placeholder state.</summary>
    public bool HasTile { get => _hasTile; private set => Set(ref _hasTile, value); }

    /// <summary>
    /// Why the card is dark, empty while it is drawing.
    ///
    /// It is one output rather than several because a reader asks one question of a dark tile.
    /// Which state it is naming is <see cref="PlaceholderFor"/>'s answer, and the states are
    /// distinct on purpose: nothing is publishing, no leg has been named, the relay has not
    /// picked the path up, the decode has been asked for, and - once there is a tile - the
    /// tile's own three (nothing is decoding this pair, the pipeline is up and no frame has
    /// left it, or this window could not draw what it was handed).
    /// </summary>
    public string Placeholder { get => _placeholder; private set => Set(ref _placeholder, value); }

    public bool HasPlaceholder { get => _hasPlaceholder; private set => Set(ref _hasPlaceholder, value); }

    /// <summary>
    /// What this picture costs to show: the relay round trip it lags by, and the downstream
    /// bandwidth it spends. Stated on the card rather than in a comment, because it is the one
    /// thing that separates this preview from the local mirror a reader would assume it is.
    /// </summary>
    public string Cost { get => _cost; private set => Set(ref _cost, value); }

    public string Encoded { get => _encoded; private set => Set(ref _encoded, value); }

    public string Quality { get => _quality; private set => Set(ref _quality, value); }

    /// <summary>
    /// Whether the inset red outline and the Program badge show. Both mean one thing -
    /// this tile is what the world is currently receiving - so both follow the same fact.
    /// </summary>
    public bool IsOnAir { get => _isOnAir; private set => Set(ref _isOnAir, value); }

    // --- Lifecycle ------------------------------------------------------------------

    /// <summary>
    /// The one render function. Sets the off branch too, so neither the outline nor the tile
    /// can stick.
    ///
    /// Safe to run twice: the converge asks for nothing when the decode it wants is the one it
    /// already asked for, the tile is reused rather than rebuilt, and every property setter
    /// compares before it notifies.
    /// </summary>
    public void Apply()
    {
        var reading = Snapshot;

        // Reconciled from the render pass rather than performed by it, the same arrangement
        // the roster has with its legs and the broadcast screen with its describe: the pass
        // states what it wants and the converge decides whether anything has to be asked.
        AskLeg();

        var wanted = Wanted();
        Converge(wanted);

        // The tile is rendered from the backend's own decode list, joined on the pair the
        // whole contract keys a decode by. A tile whose decode is not in it draws its own
        // reason for that rather than disappearing.
        _tile?.Apply(DecodeOf(_tile));

        IsOnAir = reading.IsLive;
        Encoded = $"encoded {Figure.Of(reading.Fps, "0.0")} fps";
        Quality = $"cq {Figure.Of(reading.Cq)}";
        Cost = Cards.PreviewCost(_leg.Length > 0 ? Words.Transport(_leg) : "");

        // After the tile's own pass, because a tile that is drawing has no sentence and one
        // that is not has written it by now.
        Placeholder = PlaceholderFor();
        HasPlaceholder = Placeholder.Length > 0;
        HasTile = _tile is not null;

        Assert.That(HasPlaceholder == (Placeholder.Length > 0), "a placeholder and its sentence agree", HasPlaceholder);
        Assert.That(HasTile == (Tile is not null), "a tile and the fact that there is one agree", HasTile);
        Assert.That(_tile is null || Equals(_asked, wanted), "a preview draws the decode it asked for");
        Assert.That(_tile is null || _asked is not null, "a tile on the preview belongs to a wanted decode");
        Assert.That(Cost.Length > 0, "a preview states what receiving its own stream back costs");
    }

    /// <summary>
    /// The decode this card wants open, and null for "nothing open".
    ///
    /// Four facts have to hold and each is read through rather than remembered: the card is on
    /// screen, a stream is in force and names itself, the settings have named a leg, and the
    /// relay's snapshot already carries a path by that name.
    ///
    /// The last one is a fact and not a rule. It is not a carriage verdict - which format a
    /// leg carries is the backend's answer when the decode is opened, and greying from a
    /// snapshot that can be older than the stream is exactly what the roster refuses to do.
    /// What it asks is only whether the relay has anything by this name to receive yet, which
    /// a stream that has just started does not until the next poll; without it the first
    /// seconds of every broadcast would build a pipeline against a path the relay is not
    /// serving.
    /// </summary>
    private WatchKey? Wanted()
    {
        if (!_showing)
        {
            return null;
        }

        var stream = _session.Publish?.Live?.Publish?.Name ?? "";
        if (stream.Length == 0 || _leg.Length == 0 || !Carrying(stream))
        {
            return null;
        }

        return new WatchKey { StreamName = stream, Transport = _leg };
    }

    /// <summary>
    /// Converges on the wanted decode: asks for the one that is wanted, and drops the tile
    /// belonging to one that is not.
    ///
    /// <b>The one thing it does not do is close a decode.</b> The class comment states why in
    /// full: a decode has no owner and no consumer count, so a stop from here would end a
    /// tile on the viewer screen watching the same pair.
    ///
    /// A start is asked for once per wanted decode rather than on every pass, which is the one
    /// departure from reading through. <c>StartReceive</c> is idempotent and a repeat would be
    /// correct; what a repeat per pass would not be is free, since a pass runs on every
    /// encoder sample. The guard is cleared whenever the wanted decode moves, so a decode that
    /// was refused is asked for again as soon as anything about the want changes.
    /// </summary>
    private void Converge(WatchKey? wanted)
    {
        if (Equals(_asked, wanted))
        {
            return;
        }

        _asked = wanted;
        _refusal = "";
        Draw(null);

        if (wanted is null)
        {
            return;
        }

        _ = OpenAsync(wanted);
    }

    /// <summary>
    /// Opens the decode this card draws.
    ///
    /// The tile is added once the start has answered and not before, for the reason the
    /// roster adds its own that way round: a tile is a subscription to frames and there are
    /// none until something is decoding.
    /// </summary>
    private async Task OpenAsync(WatchKey key)
    {
        try
        {
            await _backend.StartReceiveAsync(key.StreamName, key.Transport).ConfigureAwait(false);
            _dispatch(() => Opened(key, ""));
        }
        catch (BackendUnavailableException e)
        {
            // The backend's own sentence: a leg that cannot carry this stream's format names
            // the format and the protocols that would have carried it, which is the whole of
            // what makes the refusal actionable.
            _dispatch(() => Opened(key, e.Message));
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <summary>
    /// Takes what the start answered, on the UI loop, and re-renders. An answer naming a
    /// decode this card has stopped wanting is dropped: the reader went off screen or the
    /// stream moved while the call was out, and the tile it would add belongs to nothing.
    /// </summary>
    private void Opened(WatchKey key, string refusal)
    {
        if (!Equals(_asked, key))
        {
            return;
        }

        _refusal = refusal;

        // Only where there is none yet. A tile is a running subscription, so building a second
        // one for the decode this card is already drawing would restart it - and this method
        // has to be safe to reach twice for the same reason the call behind it does.
        if (refusal.Length == 0 && _tile is null)
        {
            // Nothing to rearrange: this card draws one tile with no grid around it, so focus,
            // pop-out and fullscreen have no meaning here and the intents go nowhere. The
            // preview's own template offers none of them either - it is the tile that is reused,
            // not the viewer's menu.
            Draw(new TileViewModel(key.StreamName, key.Transport, _backend, _dispatch, _ => { }));
        }

        Apply();
    }

    /// <summary>
    /// Puts one tile on the card or takes it off, and moves the subscription to its reports
    /// with it. Idempotent, and the only writer of <see cref="Tile"/>.
    /// </summary>
    private void Draw(TileViewModel? tile)
    {
        if (ReferenceEquals(_tile, tile))
        {
            return;
        }

        if (_tile is not null)
        {
            _tile.Changed -= Apply;
        }

        if (tile is not null)
        {
            // A tile reports what it drew, which no state the backend owns can carry - a
            // backend cannot see that a compositor was too slow to take a frame. The pass it
            // asks for is this card's own, so the picture and the figures over it are still
            // written by one render function.
            tile.Changed += Apply;
        }

        Tile = tile;
    }

    /// <summary>
    /// Whether the relay's snapshot carries a path by this stream's name. The join is on the
    /// name the publish state itself states, which is the same join the header figures make.
    /// </summary>
    private bool Carrying(string stream)
    {
        var relay = _session.Relay;
        if (relay is null || !relay.Reachable)
        {
            return false;
        }

        foreach (var path in relay.Paths)
        {
            if (path.Name == stream)
            {
                return true;
            }
        }

        return false;
    }

    /// <summary>
    /// The backend's state for this card's decode, and null while nothing is decoding that
    /// pair. The join is on the stream name and the leg together, because the relay re-serves
    /// each stream on all its listeners and the name alone is not an identity.
    /// </summary>
    private ReceiveStream? DecodeOf(TileViewModel tile)
    {
        foreach (var decode in _session.Receiving)
        {
            if (decode.Stream.StreamName == tile.Name && decode.Stream.Transport == tile.Transport)
            {
                return decode;
            }
        }

        return null;
    }

    /// <summary>
    /// Why the card is dark, in the order the states happen in.
    ///
    /// A tile answers for itself once there is one, because the three reasons a tile has
    /// nothing to draw are the tile's own and are already written there. Before that the
    /// states are this card's: the backend refused the start and said why, nothing is
    /// publishing, no leg has been named, the relay has not picked the path up, or the start
    /// is out. Each is a different thing to do about it, which is why none of them is folded
    /// into another.
    /// </summary>
    private string PlaceholderFor()
    {
        if (_tile is not null)
        {
            return _tile.Notice;
        }

        if (_refusal.Length > 0)
        {
            return _refusal;
        }

        var stream = _session.Publish?.Live?.Publish?.Name ?? "";
        if (stream.Length == 0)
        {
            return Cards.PreviewNotPublishing;
        }

        if (_leg.Length == 0)
        {
            return Cards.PreviewNoLeg;
        }

        if (!Carrying(stream))
        {
            return Cards.PreviewRelayHasNoPath;
        }

        return Cards.PreviewOpening;
    }

    // --- The settings the leg comes from ---------------------------------------------

    /// <summary>
    /// Asks the backend which leg a tile receives on, once.
    ///
    /// It is the value of one settings field, resolved against the settings the backend holds,
    /// and it is read exactly as the roster reads it (<see cref="TileLeg"/>). Asked once for
    /// the same reason the roster asks once: a resolve is a read and the field does not move
    /// while a window is open, and a failed read forgets that it was asked so the next pass
    /// tries again once the backend answers.
    /// </summary>
    private void AskLeg()
    {
        if (_askedLeg)
        {
            return;
        }

        _askedLeg = true;
        _ = AskLegAsync();
    }

    private async Task AskLegAsync()
    {
        try
        {
            var settings = await _backend.SettingsAsync().ConfigureAwait(false);
            var form = await _backend.ResolveFormAsync(settings).ConfigureAwait(false);

            _dispatch(() =>
            {
                _leg = TileLeg.Of(form);
                Apply();
            });
        }
        catch (BackendUnavailableException)
        {
            // The session's own reconnect reports the absence. Forgetting that it was asked
            // for is what lets the next pass ask again once the backend answers.
            _dispatch(() => _askedLeg = false);
        }
        catch (OperationCanceledException)
        {
            _dispatch(() => _askedLeg = false);
        }
    }
}
