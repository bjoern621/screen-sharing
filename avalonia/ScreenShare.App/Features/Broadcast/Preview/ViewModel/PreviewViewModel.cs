using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Features.Broadcast.Preview.Model;
using ScreenShare.App.Features.Viewer.Tile.Model;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.Preview.ViewModel;

/// <summary>
/// Outgoing preview: one stream, over one of two routes the reader picks between, or neither.
///
/// Local route: the copy the publish child writes to a loopback port, decoded on this machine.
/// End-to-end route: this machine's own stream pulled back off the relay, over the leg the viewer receives on.
/// Both carry the same encode, so what only the end-to-end route can show is the uplink, the relay and the way
/// back (<see cref="PreviewRoute"/>).
///
/// Neither route stands in for the other.
/// A route with nothing running behind it draws the placeholder naming which state it is in.
///
/// The local route costs one decode here, no bandwidth and no reader slot.
/// The end-to-end route is a relay client: a reader slot, a viewer's downstream bandwidth, and a row among the
/// viewer figures beside the card.
/// So the card opens on the local route and the other is asked for by name.
///
/// Off is a segment of that same toggle, and the card opens on a route rather than on it.
/// It does not follow the window: a publisher's window stands behind what is being shared, so a card that went
/// dark while nobody looked would be dark when a reader came back, and would pay a pool import and a reconnect to
/// return.
/// Off closes the end-to-end route's decode, which makes the segment worth more than a way to blank a tile.
///
/// Only the end-to-end route has an effect to call: <c>StartReceive</c> and <c>StopReceive</c>, as the viewer's
/// grid calls them (<see cref="Receive"/>).
/// The local pipeline goes up and down with the publish child, so nothing here converges it.
///
/// <see cref="Apply"/> converges rather than sequences, and a second pass over unchanged input asks for nothing,
/// makes nothing and drops nothing.
/// </summary>
public sealed class PreviewViewModel : Observable
{
    private readonly IBackend _backend;
    private readonly FormSession _form;
    private readonly Session _session;
    private readonly Action<Action> _dispatch;

    /// <summary>
    /// Which picture the reader asked for, <see cref="PreviewRoute.Off"/> for none.
    /// This card's own state, with nowhere to read it back from: the backend has no opinion about which of two
    /// pictures of one stream a window draws, the local pipeline belongs to the publish, and a relay decode
    /// outlives every window drawing it.
    /// A route to begin with, a card that has to be started reading as one that is broken.
    /// Written by <see cref="SelectedRoute"/> alone.
    /// </summary>
    private PreviewRoute _route = PreviewRoute.Local;

    /// <summary>
    /// Tile this card draws, null while it wants none.
    /// <see cref="TileViewModel"/> reused rather than a second frame consumer, which would be a second answer to
    /// what a dropped frame is and where a lent handle goes back.
    /// </summary>
    private TileViewModel? _tile;

    /// <summary>
    /// Relay decode this card has asked the backend for, null while it has asked for none.
    ///
    /// Shell state, and the one departure this card makes from reading everything through.
    /// The backend reports which decodes are running and never who wanted them, so the decode this card asked for
    /// cannot be derived from the running state; the viewer's grid keeps its tile list for the same reason
    /// (<c>Features/Viewer/ViewModel/ViewerViewModel.cs</c>).
    ///
    /// Holds the key that was asked for, not the one the settings name now.
    /// A stop is keyed by stream and leg together, so closing on the current setting would leave a decode running
    /// whenever the leg had moved since.
    /// </summary>
    private WatchKey? _asked;

    /// <summary>
    /// Whether a receive effect is in flight, which keeps a render pass from issuing a second behind it.
    /// The pass that runs when the answer lands converges again.
    /// </summary>
    private bool _asking;

    /// <summary>
    /// Whether the decode named by <see cref="_asked"/> has been seen running.
    ///
    /// Separates "not up yet" from "gone", which the contract cannot: the receive state says what is running and
    /// never what was.
    /// Asking again repairs a pipeline another window closed, and only duplicates an answer still on its way.
    ///
    /// Without it the converge never settles: the pass that runs when a start answers has not yet been told what
    /// is decoding, so it would read the key as gone and ask again.
    /// </summary>
    private bool _open;

    /// <summary>
    /// Why the backend refused the end-to-end decode, empty while it has not.
    /// That side's own sentence, shown as it stands: a leg that cannot carry this stream's format names the
    /// format and the protocols that would have carried it (<c>docs/ipc-api.md</c>, "Errors").
    ///
    /// Also stops the converge asking again, a refusal being a fact about this key rather than a moment.
    /// Cleared when the key moves.
    /// </summary>
    private string _refusal = "";

    /// <summary>
    /// Leg the viewer's grid is drawing one stream on, empty where it is drawing none of it.
    ///
    /// A decode is one pipeline whoever asked for it, keyed by stream and leg, so a stop issued here would take
    /// the picture out of a grid tile on the same pair.
    /// This card lets go of its ask instead and leaves the pipeline to the window that still wants it.
    ///
    /// A read of the viewer's own state rather than a copy, supplied by the shell, which holds both screens
    /// (<see cref="SetGridLeg"/>).
    /// </summary>
    private Func<string, string> _gridLeg = static _ => "";

    /// <param name="form">
    /// Settings the backend is holding, read for one value: the leg the end-to-end route receives on.
    /// Stored and not the draft, the backend building the rest of that decode out of its own settings, so a draft
    /// leg would run against kept jitter buffers (<c>Features/Viewer/Tile/Model/TileLeg.cs</c>).
    /// </param>
    /// <param name="dispatch">
    /// Hands work to the UI loop.
    /// The tile's reports and the answer to a receive effect land on whichever thread the transport completed on,
    /// and every binding here tolerates one writer thread.
    /// </param>
    public PreviewViewModel(IBackend backend, FormSession form, Session session, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a preview subscribes to the backend's frames");
        Assert.NotNull(form, "a preview reads the leg its end-to-end route receives on");
        Assert.NotNull(session, "a preview renders the session's running state");
        Assert.NotNull(dispatch, "a preview needs a UI loop to marshal a report back to");

        _backend = backend;
        _form = form;
        _session = session;
        _dispatch = dispatch;

        Routes = PreviewRoutes.All.Select(route => new PreviewRouteTab(route)).ToList();
        _selectedRoute = Routes.Single(tab => tab.Value == _route);

        Assert.That(Routes.Count == PreviewRoutes.All.Count, "a segment per route", Routes.Count);
        Assert.That(_selectedRoute.Value == _route, "the toggle opens on the route the card draws");

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
    /// Says which leg the viewer's grid draws a given stream on, so the end-to-end route does not close a decode
    /// that window is still drawing.
    /// Idempotent: it replaces a read, and rendering after it converges to the same world.
    /// A function rather than a value, so the answer is read when it matters instead of copied when the shell
    /// happened to call this.
    /// The default answers that nothing else is drawing anything, what a card with no shell around it gets.
    /// </summary>
    public void SetGridLeg(Func<string, string> legOf)
    {
        Assert.NotNull(legOf, "a shared decode is read through the window that also draws it");

        _gridLeg = legOf;
        Apply();
    }

    // --- Outputs ------------------------------------------------------------------

    private string _placeholder = "";
    private bool _hasPlaceholder;
    private string _cost = "";
    private string _encoded = "";
    private string _quality = "";
    private string _leg = "";
    private bool _hasLeg;
    private bool _isSharing;
    private bool _hasTile;
    private PreviewRouteTab _selectedRoute;

    /// <summary>
    /// Tile the picture is drawn from, null while there is none.
    /// The control bound to it opens the frame subscription on attach and cancels it on detach, so clearing this
    /// ends the subscription (<c>Features/Viewer/Tile/View/StreamTile.cs</c>).
    /// Reads the field <see cref="Draw"/> writes: one tile, one owner.
    /// </summary>
    public TileViewModel? Tile { get => _tile; private set => Set(ref _tile, value); }

    public bool HasTile { get => _hasTile; private set => Set(ref _hasTile, value); }

    /// <summary>A segment per route, in <see cref="PreviewRoutes.All"/> order. Fixed for the card's life.</summary>
    public IReadOnlyList<PreviewRouteTab> Routes { get; }

    /// <summary>
    /// What the route toggle has selected.
    /// The reader owns it, so the setter is the write and the render function follows.
    /// Settable where every other output here is not, the control writing it back on selection.
    /// Idempotent: selecting the segment already selected notifies nothing.
    /// </summary>
    public PreviewRouteTab SelectedRoute
    {
        get => _selectedRoute;
        set
        {
            // A list box clears its selection while its items are replaced, and null here would leave the card
            // drawing no route at all.
            if (value is null || ReferenceEquals(value, _selectedRoute))
            {
                return;
            }

            Set(ref _selectedRoute, value);
            _route = value.Value;

            // The refusal belonged to the route being left.
            // Kept across the switch, it would describe a leg the other route never uses.
            _refusal = "";
            Apply();
        }
    }

    public string RouteChoice => Cards.PreviewRouteChoice;

    /// <summary>
    /// Why the card is dark, empty while it is drawing.
    /// One output rather than several, a reader asking one question of a dark tile.
    /// Which state it names is <see cref="PlaceholderFor"/>'s answer.
    /// </summary>
    public string Placeholder { get => _placeholder; private set => Set(ref _placeholder, value); }

    public bool HasPlaceholder { get => _hasPlaceholder; private set => Set(ref _hasPlaceholder, value); }

    /// <summary>
    /// What this picture is and is not, in the chosen route's own words: where it was taken, what it costs, and
    /// which question it cannot answer.
    /// On the card rather than in a comment, a preview that looks perfect while viewers suffer being the
    /// misreading it exists to prevent.
    /// One sentence for both routes would be false under one of them, the two making opposite claims.
    /// </summary>
    public string Cost { get => _cost; private set => Set(ref _cost, value); }

    public string Encoded { get => _encoded; private set => Set(ref _encoded, value); }

    public string Quality { get => _quality; private set => Set(ref _quality, value); }

    /// <summary>
    /// Protocol the picture crossed, empty on the route that crossed none.
    /// The leg the decode was opened on rather than the one the settings name now, read off the tile for the same
    /// reason a stop is.
    /// </summary>
    public string Leg { get => _leg; private set => Set(ref _leg, value); }

    public bool HasLeg { get => _hasLeg; private set => Set(ref _hasLeg, value); }

    /// <summary>
    /// Whether the inset red outline and the sharing badge show.
    /// Both mean "this tile is what the world is receiving", so both follow one fact.
    /// </summary>
    public bool IsSharing { get => _isSharing; private set => Set(ref _isSharing, value); }

    private bool _hasPointer;
    private double _pointerLeft;
    private double _pointerTop;
    private double _pictureWidth;
    private double _pictureHeight;

    /// <summary>
    /// Whether the publish is sending a pointer position at all.
    /// False for every cursor mode but the one that sends it, and false while the pointer is off the captured
    /// screen: a pointer that has left is not at its last position.
    /// Drawn on both routes, the position travelling beside the picture rather than in it.
    /// </summary>
    public bool HasPointer { get => _hasPointer; private set => Set(ref _hasPointer, value); }

    /// <summary>
    /// Where the marker sits over the picture, in the rendered card's own pixels.
    /// The backend sends the position in the picture's pixels, so the one conversion is the fraction of the way
    /// across times the size this card is drawn at.
    /// Here and not in the view, which does not know the picture's own size.
    /// </summary>
    public double PointerLeft { get => _pointerLeft; private set => Set(ref _pointerLeft, value); }

    public double PointerTop { get => _pointerTop; private set => Set(ref _pointerTop, value); }

    /// <summary>
    /// Size this card is drawing the picture at, written by the view as it lays out.
    /// The one fact the view knows and the view model cannot read, hence the one input the view writes.
    /// </summary>
    public void SetPictureSize(double width, double height)
    {
        _pictureWidth = width;
        _pictureHeight = height;
        Point(_session.Pointer);
    }

    /// <summary>
    /// Takes one pointer position, or none.
    /// Its own entry point rather than part of <see cref="Apply"/>: positions arrive hundreds of times a second on
    /// a stream of their own, and a render pass each would re-read the whole session to move one marker
    /// (<c>Backend/Session.cs</c>, <c>Metered</c>).
    /// </summary>
    public void Point(PointerPosition? at)
    {
        var picture = Tile;
        var drawn = _pictureWidth > 0 && _pictureHeight > 0;
        if (at is null || !at.Visible || picture is null || !drawn)
        {
            HasPointer = false;
            return;
        }

        if (picture.PictureWidth <= 0 || picture.PictureHeight <= 0)
        {
            HasPointer = false;
            return;
        }

        // Centred on the position, the marker standing for a point rather than a box.
        HasPointer = true;
        PointerLeft = (at.X * _pictureWidth / picture.PictureWidth) - (PointerSize / 2);
        PointerTop = (at.Y * _pictureHeight / picture.PictureHeight) - (PointerSize / 2);
    }

    /// <summary>Marker width in px, drawn at this size by the view and centred by this.</summary>
    private const double PointerSize = 14;

    // --- Lifecycle ------------------------------------------------------------------

    /// <summary>
    /// The one render function.
    /// Sets the off branch of everything it writes, so neither the outline nor the tile nor a refusal sticks.
    /// Safe to run twice: the receive converge asks for nothing already asked, the tile converge keeps the
    /// tile drawing the same picture, and every setter compares before it notifies.
    /// </summary>
    public void Apply()
    {
        var reading = Snapshot;

        // The relay decode first, the tile being built from what is running: a subscription opened before its
        // decode is refused once and never retries.
        var wanted = Wanted();
        Receive(wanted);

        var pipeline = Running(wanted);
        Converge(pipeline is null ? null : SourceFor(wanted));

        // Rendered from what the backend reports about the pipeline behind it, read through on every pass, so a
        // tile whose pipeline is absent draws its own reason rather than disappearing.
        // The sample is the end-to-end decode's and nothing on the local route, this card drawing no stats panel
        // either way.
        _tile?.Apply(pipeline, wanted is null ? null : _session.StatsOf(wanted.StreamName, wanted.Transport));

        IsSharing = reading.IsLive;
        Encoded = $"encoded {Figure.Of(reading.Fps, "0.0")} fps";
        Quality = $"cq {Figure.Of(reading.Cq)}";
        Cost = PreviewRoutes.CostOf(_route);

        // Only the end-to-end route has a leg and the local route's tile reports the empty string, so there is no
        // case on the route here.
        Leg = _tile is { Transport.Length: > 0 } tile ? Words.Transport(tile.Transport) : "";
        HasLeg = Leg.Length > 0;

        // After the tile's own pass: a tile that is drawing has no sentence, and one that is not has written it by
        // now.
        Placeholder = PlaceholderFor();
        HasPlaceholder = Placeholder.Length > 0;
        HasTile = _tile is not null;

        Assert.That(HasPlaceholder == (Placeholder.Length > 0), "a placeholder and its sentence agree", HasPlaceholder);
        Assert.That(HasLeg == (Leg.Length > 0), "a leg and the fact that there is one agree", Leg);
        Assert.That(HasTile == (Tile is not null), "a tile and the fact that there is one agree", HasTile);
        Assert.That(_tile is null || pipeline is not null, "a preview tile draws a picture something is producing");
        Assert.That(
            _route == PreviewRoute.EndToEnd || _asked is null,
            "only the end-to-end route holds a relay decode open", (int)_route);
        Assert.That(_route != PreviewRoute.Off || _tile is null, "a preview that is off draws no tile");
        Assert.That(Cost.Length > 0, "a preview states what it is showing and what it is not");
        Assert.That(SelectedRoute.Value == _route, "the toggle and the picture name one route", (int)_route);
    }

    /// <summary>
    /// Relay decode the end-to-end route needs, null where it needs none: the local route, the off segment, and
    /// nothing publishing.
    /// The stream and the leg are read through on every pass, the route being the card's own.
    /// The stream is the publish's own name, this route receiving this machine's stream rather than one a reader
    /// chose.
    /// The leg is the viewer's setting, a second one here being a second answer to how this machine watches.
    /// </summary>
    private WatchKey? Wanted()
    {
        if (_route != PreviewRoute.EndToEnd)
        {
            return null;
        }

        var stream = Publishing();
        var leg = TileLeg.Of(_form.Stored);
        if (stream.Length == 0 || leg.Length == 0)
        {
            return null;
        }

        return new WatchKey { StreamName = stream, Transport = leg };
    }

    /// <summary>
    /// Name this machine is publishing under, empty where it is publishing nothing.
    /// A live publish always names itself, so the empty string is a state no tile is made for rather than one
    /// drawn with a blank heading.
    /// </summary>
    private string Publishing() => _session.Publish?.Live?.Publish?.Name ?? "";

    /// <summary>
    /// Picture the chosen route has running behind it, null for nothing to draw.
    /// One fact is read through on either route: something is producing the picture.
    /// What produces it differs, the local route's pipeline being part of the publish and the end-to-end route's a
    /// decode in the receive state, and reading both into one shape lets one tile draw either
    /// (<c>Features/Viewer/Tile/Model/TilePipeline.cs</c>).
    /// </summary>
    private TilePipeline? Running(WatchKey? wanted)
    {
        if (_route == PreviewRoute.Off)
        {
            return null;
        }

        return _route == PreviewRoute.EndToEnd
            ? TilePipeline.Of(wanted is null ? null : Decoding(wanted))
            : TilePipeline.Of(_session.Publish?.Live?.Preview);
    }

    /// <summary>
    /// Decode the backend reports for one key, null where it is running none.
    /// Read out of the whole receive state on every pass, a decode the viewer's grid opened on the same pair
    /// being the same pipeline and drawing the same picture.
    /// </summary>
    private ReceiveStream? Decoding(WatchKey key)
    {
        foreach (var decode in _session.Receiving)
        {
            if (decode.Stream.StreamName == key.StreamName && decode.Stream.Transport == key.Transport)
            {
                return decode;
            }
        }

        return null;
    }

    /// <summary>
    /// What a tile on the chosen route subscribes to, null where this card has no name to build one with.
    /// The contract's own distinction: the running publish's preview, or a stream and a leg
    /// (<c>Features/Viewer/Tile/Model/TileSource.cs</c>).
    /// </summary>
    private TileSource? SourceFor(WatchKey? wanted)
    {
        if (_route == PreviewRoute.EndToEnd)
        {
            return wanted is null ? null : TileSource.Relay(wanted.StreamName, wanted.Transport);
        }

        var stream = Publishing();
        return stream.Length == 0 ? null : TileSource.Preview(stream);
    }

    /// <summary>
    /// Converges the backend onto the decode this card wants: opens the one the end-to-end route needs, closes
    /// the one it has stopped needing.
    ///
    /// Idempotent, which makes it safe on a render pass.
    /// A key already asked for and running asks nothing, a refused key asks nothing until it moves, and one call
    /// is in flight at a time, so a hundred passes cost the first pass's round trip.
    ///
    /// A decode that went away is asked for again, decodes being shared and outliving the window that opened one.
    /// The repair is for a pipeline seen running and now gone, what <see cref="_open"/> is held for.
    /// </summary>
    private void Receive(WatchKey? want)
    {
        // The decode this card no longer wants goes first, so a route switch or a moved leg cannot leave two open.
        if (_asked is not null && !Names(_asked, want))
        {
            var stale = _asked;
            _asked = null;
            _open = false;
            _refusal = "";
            Release(stale);
        }

        if (want is null || _asking)
        {
            return;
        }

        if (Names(_asked, want))
        {
            if (Decoding(want) is not null)
            {
                _open = true;
                return;
            }

            // Asked for and not running: the answer is still out, or the leg refused.
            // Both are waited out rather than asked again.
            if (!_open || _refusal.Length > 0)
            {
                return;
            }
        }

        _asked = want;
        _open = false;
        _asking = true;
        _ = OpenAsync(want);
    }

    /// <summary>Whether two keys name one decode, false where either is absent.</summary>
    private static bool Names(WatchKey? key, WatchKey? other)
        => key is not null && other is not null
            && key.StreamName == other.StreamName && key.Transport == other.Transport;

    /// <summary>
    /// Asks the relay for a decode of this machine's own stream, and holds the refusal where there is one.
    /// Nothing is written about what the decode became: the answer carries no state and what is running arrives
    /// on the event stream.
    /// What lands here is whether there is a sentence to show, and that the next pass may ask again.
    /// </summary>
    private async Task OpenAsync(WatchKey key)
    {
        var refusal = "";
        try
        {
            await _backend.StartReceiveAsync(key.StreamName, key.Transport).ConfigureAwait(false);
        }
        catch (BackendUnavailableException e)
        {
            // The backend's own sentence: a leg that cannot carry this stream's format names the format and the
            // protocols that would have carried it.
            refusal = e.Message;
        }
        catch (OperationCanceledException)
        {
        }

        _dispatch(() =>
        {
            _asking = false;

            if (Names(_asked, key))
            {
                _refusal = refusal;
            }
            else if (refusal.Length == 0)
            {
                // The route was toggled while this was in flight, so the key is already let go, and the stop that
                // went with it may have reached the backend before this start did.
                // Letting go a second time closes what nothing wants, and costs nothing where the first one
                // already did: a stop naming a decode that is not running is a success.
                Release(key);
            }

            Apply();
        });
    }

    /// <summary>
    /// Lets go of one decode, and closes it unless the viewer's grid is drawing the same pair.
    /// A decode is one pipeline whoever asked for it, keyed by stream and leg, so a stop issued here takes the
    /// picture out of every window drawing it.
    /// Where the grid holds the same pair, the pipeline is left to it and the grid's own stop closes it
    /// (<see cref="SetGridLeg"/>).
    /// </summary>
    private void Release(WatchKey key)
    {
        if (_gridLeg(key.StreamName) == key.Transport)
        {
            return;
        }

        _ = CloseAsync(key);
    }

    private async Task CloseAsync(WatchKey key)
    {
        try
        {
            await _backend.StopReceiveAsync(key.StreamName, key.Transport).ConfigureAwait(false);
        }
        catch (BackendUnavailableException)
        {
            // A decode this card cannot reach is one it cannot close, and it has already stopped drawing it.
            // The backend's own roster answers for it from here, as it does for a shell that crashed.
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <summary>
    /// Converges on the picture that is running: makes the tile where there is one to draw, drops it where there
    /// is not.
    /// A tile naming the same picture is kept, rebuilding one restarting a subscription already drawing.
    /// The comparison is the whole source and not the stream name alone, so an end-to-end route whose leg moved
    /// is rebuilt on the leg it now receives over.
    /// </summary>
    private void Converge(TileSource? source)
    {
        if (source is null)
        {
            Draw(null);
            return;
        }

        if (_tile is not null && _tile.Source == source)
        {
            return;
        }

        // Nothing to rearrange: one tile with no grid around it, so focus, pop-out and fullscreen mean nothing here
        // and the intents go nowhere.
        // The card's template offers none of them either, the tile being what is reused rather than the viewer's
        // menu.
        Draw(new TileViewModel(source, _backend, _dispatch, _ => { }));
    }

    /// <summary>
    /// Puts one tile on the card or takes it off, moving the subscription to its reports with it.
    /// Idempotent, and the only writer of <see cref="Tile"/>.
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
            // A tile reports what it drew, which no backend state carries: a backend cannot see that a compositor
            // was too slow to take a frame.
            // The pass it asks for is this card's own, so one render function still writes the picture and the
            // figures over it.
            tile.Changed += Apply;
        }

        Tile = tile;
    }

    /// <summary>
    /// Why the card is dark, in the order a reader can act on.
    /// The off segment comes first, being the reader's own doing: a card off over an unpublished stream is off.
    /// A refusal comes next, the one sentence naming something to change, where every state under it is answered
    /// by waiting.
    /// A tile answers for itself after those, the three reasons it has nothing to draw being written there
    /// already.
    /// What is left is this card's, the last of them the chosen route's, the two routes being dark for different
    /// reasons.
    /// </summary>
    private string PlaceholderFor()
    {
        if (_route == PreviewRoute.Off)
        {
            return Cards.PreviewOff;
        }

        if (_refusal.Length > 0)
        {
            return _refusal;
        }

        if (_tile is not null)
        {
            return _tile.Notice;
        }

        if (_session.Publish?.Live is null)
        {
            return Cards.PreviewNotPublishing;
        }

        if (_route != PreviewRoute.EndToEnd)
        {
            return Cards.PreviewNotPreviewed;
        }

        return TileLeg.Of(_form.Stored).Length == 0 ? Cards.PreviewNoWatchLeg : Cards.PreviewOpening;
    }
}
