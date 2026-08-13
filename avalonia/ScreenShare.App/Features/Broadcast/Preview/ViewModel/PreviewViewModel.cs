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
/// The outgoing preview: what is being sent, and what it costs to show it.
///
/// <b>It draws one of two pictures of one stream, and the reader picks which.</b> The local route is a copy
/// the publish child writes to a loopback port and this machine decodes; the end-to-end route is this
/// machine's own stream pulled back off the relay over the leg the viewer receives on.
/// They differ by where the picture is taken and by nothing else - both carry the same encode - so what one
/// shows and the other cannot is the uplink, the relay and the way back (<see cref="PreviewRoute"/>).
///
/// <b>The routes are not interchangeable and the card never substitutes one for the other.</b> A publish with
/// no local preview leg draws nothing on the local route, a relay that is not carrying the path draws nothing
/// on the end-to-end one, and each state says which it is.
/// The sentence under the card is the route's own, so the picture and the claim made about it cannot disagree
/// (<see cref="Cost"/>).
///
/// <b>What each route costs is different, and that is the reason the toggle is a choice rather than a
/// setting.</b> The local route costs one decode on this machine, spends no bandwidth, and the relay serves
/// no reader for it - the viewer figures beside this card are viewers, with nothing of this machine's own in
/// them.
/// The end-to-end route is a relay client like any tile in the grid: it takes a reader slot, it is counted
/// among those figures, and it pays a viewer's downstream bandwidth.
/// So the card opens on the local route and the other is asked for by name.
///
/// <b>Only one of the two routes has an effect to call.</b> The local preview pipeline goes up with the
/// publish child and down with it, so there is nothing here to converge on beyond the subscription.
/// The end-to-end route opens a decode with <c>StartReceive</c> and closes it with <c>StopReceive</c>,
/// exactly as the viewer's grid does, which is the one place this card asks the backend for anything
/// (<see cref="Receive"/>).
///
/// <b>The lifecycle is a converge and not a sequence.</b> <see cref="Apply"/> states the world it wants - a
/// decode where the chosen route needs one, and a tile subscribed to whatever is running - and the converges
/// decide what has to be asked for or dropped.
/// A second pass over unchanged input asks for nothing, makes nothing and drops nothing.
/// </summary>
public sealed class PreviewViewModel : Observable
{
    private readonly IBackend _backend;
    private readonly FormSession _form;
    private readonly Session _session;
    private readonly Action<Action> _dispatch;

    /// <summary>
    /// Whether this card is being looked at: it stands in a visual tree, in a window that is in front of the
    /// reader.
    /// Both halves are facts the view knows and the view model cannot read: the shell renders every
    /// destination on every pass, so without them a reader who never opened this screen, or whose window has
    /// been behind another for an hour, would still be paying for the frame traffic and the GPU copies of a
    /// picture nobody is looking at.
    /// On the end-to-end route it is what the relay reader slot is paid for too, which is why leaving the
    /// screen closes the decode rather than merely stopping the drawing.
    /// Written by <see cref="SetShowing"/> and by nothing else.
    /// </summary>
    private bool _showing;

    /// <summary>
    /// Which picture the reader asked for.
    /// It is this card's own state and there is nowhere to read it back from: the backend has no opinion
    /// about which of two pictures of one stream a window draws.
    /// Written by <see cref="SelectedRoute"/> and by nothing else.
    /// </summary>
    private PreviewRoute _route = PreviewRoute.Local;

    /// <summary>
    /// The tile this card draws, and null while it wants none.
    ///
    /// It is <see cref="TileViewModel"/> and not a second frame consumer written here.
    /// A tile is a source, the figures over it and the subscription the control behind it reads, and this
    /// card is one of those - a second implementation would be a second answer to what a dropped frame is and
    /// where a lent handle goes back.
    /// </summary>
    private TileViewModel? _tile;

    /// <summary>
    /// The relay decode this card has asked the backend for, and null while it has asked for none.
    ///
    /// <b>It is the shell's own state, which is the one departure this card makes from reading everything
    /// through.</b> The backend reports which decodes are running and says nothing about who wanted them, so
    /// "the decode this card asked for" cannot be derived from the running state - the viewer's grid keeps
    /// its tile list for the same reason (<c>Features/Viewer/ViewModel/ViewerViewModel.cs</c>).
    ///
    /// It holds the key that was asked for rather than the one the settings name now.
    /// A stop is keyed by the stream and the leg together, so closing on the current setting would leave a
    /// decode running whenever the leg had moved since it was opened.
    /// </summary>
    private WatchKey? _asked;

    /// <summary>
    /// Whether a receive effect is in flight, which is what keeps a render pass from issuing a second one
    /// behind the first.
    /// The pass that runs when the answer lands converges again.
    /// </summary>
    private bool _asking;

    /// <summary>
    /// Whether the decode named by <see cref="_asked"/> has been seen running.
    ///
    /// <b>It is what separates "not up yet" from "gone", and the converge needs both.</b> A key that has been
    /// asked for and is not in the receive state is one of two things: an answer still on its way, which
    /// asking again would only duplicate, or a pipeline another window closed, which asking again is exactly
    /// the repair for.
    /// Nothing on the contract tells them apart - the receive state says what is running and never what was -
    /// so the one bit that does is held here.
    ///
    /// Without it the converge cannot settle: the pass that runs when a start answers has not yet been told
    /// what is decoding, so it would read the key as gone and ask again, and the answer to that would do the
    /// same.
    /// </summary>
    private bool _open;

    /// <summary>
    /// Why the backend refused to open the end-to-end decode, empty while it has not.
    /// It is that side's own sentence and is shown as it stands: a leg that cannot carry this stream's format
    /// names the format and the protocols that would have carried it, which is the whole of what makes the
    /// refusal actionable (<c>docs/ipc-api.md</c>, "Errors").
    ///
    /// It also stops the converge from asking again.
    /// A refusal is a fact about this key rather than a moment, so re-asking on every render pass would be a
    /// round trip a second against a leg that has already answered; it is cleared when the key moves.
    /// </summary>
    private string _refusal = "";

    /// <summary>
    /// The leg the viewer's grid is drawing one stream on, and the empty string where it is drawing none of
    /// it.
    ///
    /// <b>It exists because a decode is one pipeline whoever asked for it.</b> The relay decode this card
    /// opens is keyed by the stream and the leg, and a tile in the viewer's grid on the same pair is the same
    /// decode - so a stop issued here would take the picture out of that tile.
    /// This card therefore lets go of its ask and leaves the pipeline to the window that still wants it.
    ///
    /// It is a read of the viewer's own state rather than a copy of it, which is what keeps there being one
    /// answer to which streams are tiled.
    /// The shell supplies it, because the shell is what holds both screens (<see cref="SetGridLeg"/>).
    /// </summary>
    private Func<string, string> _gridLeg = static _ => "";

    /// <param name="form">
    /// The settings the backend is holding, read for one value: the leg the end-to-end route receives on.
    /// Stored and not the draft, for the reason the viewer's grid reads the stored ones - the backend builds
    /// the rest of that decode out of its own settings, so opening on a draft would run an unkept leg against
    /// kept jitter buffers (<c>Features/Viewer/Tile/Model/TileLeg.cs</c>).
    /// </param>
    /// <param name="dispatch">
    /// Hands work to the UI loop.
    /// The tile's own reports and the answer to a receive effect land on whichever thread the transport
    /// completed on, and everything this writes is read by a binding that only tolerates being written from
    /// one.
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
        _selectedRoute = Routes[0];

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
    /// Says whether the card is being looked at.
    /// The named write of that state, and idempotent: telling it what it already holds re-renders and
    /// converges to the same world.
    ///
    /// The view calls it, because whether a control is in a visual tree and whether the window around it is
    /// in front are facts only the control and the platform can see.
    /// Nothing else writes it, and no render pass reads a toolkit to find it out for itself.
    /// </summary>
    public void SetShowing(bool showing)
    {
        _showing = showing;
        Apply();
    }

    /// <summary>
    /// Says which leg the viewer's grid draws a given stream on, so the end-to-end route does not close a
    /// decode that window is still drawing.
    /// The named write of the one fact this card cannot read for itself, and idempotent - it replaces a read,
    /// and rendering after it converges to the same world.
    ///
    /// A function rather than a value, so the answer is read at the moment it matters instead of copied when
    /// the shell happened to call this.
    /// The default answers that nothing else is drawing anything, which is what a card built with no shell
    /// around it - every test - gets.
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
    /// The tile the picture is drawn from, and null while there is none.
    /// The control bound to it opens the frame subscription when it is attached and cancels it when it is
    /// not, so clearing this is what ends the subscription (<c>Features/Viewer/Tile/View/StreamTile.cs</c>).
    ///
    /// It reads the same field <see cref="Draw"/> writes, rather than a second copy of it: one tile, one
    /// owner.
    /// </summary>
    public TileViewModel? Tile { get => _tile; private set => Set(ref _tile, value); }

    /// <summary>Whether a tile is on the card, which is what separates it from its placeholder state.</summary>
    public bool HasTile { get => _hasTile; private set => Set(ref _hasTile, value); }

    /// <summary>One segment per route, in the table's order. Fixed for the card's life.</summary>
    public IReadOnlyList<PreviewRouteTab> Routes { get; }

    /// <summary>
    /// What the route toggle has selected.
    /// The reader owns it, so its setter is the write and the render function follows rather than the other
    /// way round.
    ///
    /// Idempotent: selecting the segment that is already selected notifies nothing and converges to the same
    /// world.
    /// The control writes it back on selection, which is why it is settable where every other output here is
    /// not.
    /// </summary>
    public PreviewRouteTab SelectedRoute
    {
        get => _selectedRoute;
        set
        {
            // A list box clears its selection while its items are being replaced, and a null here would be a
            // card drawing no route at all.
            if (value is null || ReferenceEquals(value, _selectedRoute))
            {
                return;
            }

            Set(ref _selectedRoute, value);
            _route = value.Value;

            // The refusal belonged to the route being left.
            // Held across the switch, it would stand under the other route's picture describing a leg that
            // route never uses.
            _refusal = "";
            Apply();
        }
    }

    /// <summary>What the toggle chooses, said above it. Fixed, and read straight off the copy table.</summary>
    public string RouteChoice => Cards.PreviewRouteChoice;

    /// <summary>
    /// Why the card is dark, empty while it is drawing.
    ///
    /// It is one output rather than several because a reader asks one question of a dark tile.
    /// Which state it is naming is <see cref="PlaceholderFor"/>'s answer, and the states are distinct on
    /// purpose: the relay refused the decode, nothing is publishing, the chosen route has nothing running
    /// behind it, and - once there is a tile - the tile's own three.
    /// </summary>
    public string Placeholder { get => _placeholder; private set => Set(ref _placeholder, value); }

    public bool HasPlaceholder { get => _hasPlaceholder; private set => Set(ref _hasPlaceholder, value); }

    /// <summary>
    /// What this picture is and is not, in the chosen route's own words: where it was taken, what it costs,
    /// and which question it cannot answer.
    /// Stated on the card rather than in a comment, because a preview that looks perfect while viewers suffer
    /// is exactly the misreading it exists to prevent - and because the two routes make opposite claims, so a
    /// single sentence for both would be false under one of them.
    /// </summary>
    public string Cost { get => _cost; private set => Set(ref _cost, value); }

    public string Encoded { get => _encoded; private set => Set(ref _encoded, value); }

    public string Quality { get => _quality; private set => Set(ref _quality, value); }

    /// <summary>
    /// The protocol the picture crossed, as the strip over it prints it, and empty on the route that crossed
    /// none.
    /// It is the leg the decode was actually opened on rather than the one the settings name now, which is
    /// the tile's own answer for the same reason a stop reads it from there.
    /// </summary>
    public string Leg { get => _leg; private set => Set(ref _leg, value); }

    public bool HasLeg { get => _hasLeg; private set => Set(ref _hasLeg, value); }

    /// <summary>
    /// Whether the inset red outline and the sharing badge show.
    /// Both mean one thing - this tile is what the world is currently receiving - so both follow the same
    /// fact.
    /// </summary>
    public bool IsSharing { get => _isSharing; private set => Set(ref _isSharing, value); }

    private bool _hasPointer;
    private double _pointerLeft;
    private double _pointerTop;
    private double _pictureWidth;
    private double _pictureHeight;

    /// <summary>
    /// Whether the publish is sending a pointer position at all.
    ///
    /// False for every cursor mode but the one that sends it, and false while the pointer is off the captured
    /// screen: a pointer that has left is not at its last position, and drawing it there would leave one
    /// stuck against an edge for as long as it is away.
    ///
    /// It is drawn on both routes.
    /// The position travels beside the picture rather than in it, so neither route's frames carry a pointer
    /// and both need the marker drawn over them.
    /// </summary>
    public bool HasPointer { get => _hasPointer; private set => Set(ref _hasPointer, value); }

    /// <summary>
    /// Where the marker sits over the picture, in the rendered card's own pixels.
    ///
    /// The backend sends the position in the picture's pixels, so this is the one conversion: the fraction of
    /// the way across, times the size this card is being drawn at.
    /// It is done here and not in the view because the view has no idea what the picture's own size is, and
    /// it is done at all because a viewer's pixels are never the publisher's.
    /// </summary>
    public double PointerLeft { get => _pointerLeft; private set => Set(ref _pointerLeft, value); }

    public double PointerTop { get => _pointerTop; private set => Set(ref _pointerTop, value); }

    /// <summary>
    /// The size this card is drawing the picture at, written by the view as it lays out.
    ///
    /// It is the one fact the view knows and the view model cannot read, which is the same reason
    /// <see cref="SetShowing"/> exists: a marker placed without it would be placed on a picture whose size
    /// nothing here had measured.
    /// </summary>
    public void SetPictureSize(double width, double height)
    {
        _pictureWidth = width;
        _pictureHeight = height;
        Point(_session.Pointer);
    }

    /// <summary>
    /// Takes one pointer position, or none.
    ///
    /// <b>Its own entry point, and not part of <see cref="Apply"/>.</b> Positions arrive hundreds of times a
    /// second on a stream of their own, and running the render pass at that rate would re-read the whole
    /// session to move one marker (<c>Backend/Session.cs</c>, <c>Metered</c>).
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

        // Centred on the position rather than hung off its top left, because what the marker stands for is a
        // point and not a box.
        HasPointer = true;
        PointerLeft = (at.X * _pictureWidth / picture.PictureWidth) - (PointerSize / 2);
        PointerTop = (at.Y * _pictureHeight / picture.PictureHeight) - (PointerSize / 2);
    }

    /// <summary>How wide the marker is drawn, which the view states and this centres by.</summary>
    private const double PointerSize = 14;

    // --- Lifecycle ------------------------------------------------------------------

    /// <summary>
    /// The one render function.
    /// Sets the off branch of everything it writes, so neither the outline nor the tile nor a refusal can
    /// stick.
    ///
    /// Safe to run twice: the receive converge asks for nothing it has already asked for, the tile converge
    /// keeps the tile it already has for the picture it is already drawing, and every property setter
    /// compares before it notifies.
    /// </summary>
    public void Apply()
    {
        var reading = Snapshot;

        // The relay decode first, because the tile is built from what is running: a subscription opened
        // before its decode is refused once and never retries, which is the same order the viewer's grid
        // opens a tile in (Features/Viewer/ViewModel/ViewerViewModel.cs).
        var wanted = Wanted();
        Receive(wanted);

        var pipeline = Running(wanted);
        Converge(pipeline is null ? null : SourceFor(wanted));

        // The tile is rendered from the state the backend reports about the pipeline behind it, read through
        // on every pass.
        // A tile whose pipeline is not in the state draws its own reason for that rather than disappearing.
        //
        // The sample beside it is the end-to-end route's decode, and nothing on the local route: this card
        // draws no stats panel either way, and handing a tile the sample its key names is what keeps that a
        // fact about the card rather than about the tile.
        _tile?.Apply(pipeline, wanted is null ? null : _session.StatsOf(wanted.StreamName, wanted.Transport));

        IsSharing = reading.IsLive;
        Encoded = $"encoded {Figure.Of(reading.Fps, "0.0")} fps";
        Quality = $"cq {Figure.Of(reading.Cq)}";
        Cost = PreviewRoutes.CostOf(_route);

        // The leg the picture actually crossed, off the tile that is drawing it.
        // Only the end-to-end route has one, and the local route's tile reports the empty string, so there is
        // no case for the route here.
        Leg = _tile is { Transport.Length: > 0 } tile ? Words.Transport(tile.Transport) : "";
        HasLeg = Leg.Length > 0;

        // After the tile's own pass, because a tile that is drawing has no sentence and one that is not has
        // written it by now.
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
        Assert.That(Cost.Length > 0, "a preview states what it is showing and what it is not");
        Assert.That(SelectedRoute.Value == _route, "the toggle and the picture name one route", (int)_route);
    }

    /// <summary>
    /// The relay decode the end-to-end route needs, and null where it needs none - which is every pass on the
    /// local route, every pass with the card off screen, and every pass with nothing publishing.
    ///
    /// All three facts are read through rather than remembered.
    /// The stream is the publish's own name, because what this route receives is this machine's stream and
    /// not a stream a reader chose; the leg is the viewer's, because how this machine watches is one setting
    /// and a second one here would be a second answer to it.
    /// </summary>
    private WatchKey? Wanted()
    {
        if (!_showing || _route != PreviewRoute.EndToEnd)
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
    /// The name this machine is publishing under, and the empty string where it is publishing nothing.
    /// A publish that is live always names itself, so the empty string is a state no tile is made for rather
    /// than one drawn with a blank heading.
    /// </summary>
    private string Publishing() => _session.Publish?.Live?.Publish?.Name ?? "";

    /// <summary>
    /// The picture the chosen route has running behind it, and null for "nothing to draw".
    ///
    /// Two facts have to hold on either route and each is read through rather than remembered: the card is
    /// being looked at, and something is producing the picture.
    /// What that something is differs - the local route's pipeline is part of the publish, and the end-to-end
    /// route's is a decode in the receive state - and reading both into one shape here is what lets one tile
    /// draw either (<c>Features/Viewer/Tile/Model/TilePipeline.cs</c>).
    /// </summary>
    private TilePipeline? Running(WatchKey? wanted)
    {
        if (!_showing)
        {
            return null;
        }

        return _route == PreviewRoute.EndToEnd
            ? TilePipeline.Of(wanted is null ? null : Decoding(wanted))
            : TilePipeline.Of(_session.Publish?.Live?.Preview);
    }

    /// <summary>
    /// The decode the backend reports for one key, and null where it is running none.
    /// Read out of the whole receive state on every pass, because a decode this card did not open - the
    /// viewer's grid opened the same pair - is the same pipeline and draws the same picture.
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
    /// What a tile on the chosen route subscribes to, and null where this card has no name to build one with.
    /// It is the contract's own distinction rather than a second one: the local route names the running
    /// publish's preview and the end-to-end route names a stream and a leg
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
    /// Converges the backend onto the decode this card wants: opens the one the end-to-end route needs, and
    /// closes the one it has stopped needing.
    ///
    /// <b>Idempotent, which is what makes it safe on a render pass.</b> A key that has already been asked for
    /// and is running asks nothing, a key that was refused asks nothing until it moves, and one call is in
    /// flight at a time.
    /// Rendering a hundred times therefore costs the one round trip the first pass made.
    ///
    /// <b>A decode that went away is asked for again.</b> Decodes are shared and outlive the window that
    /// opened one, so another window can close the pipeline this card is drawing; re-asking is what makes
    /// that a blink rather than a card that stays dark until the route is toggled.
    /// It is a repair for a pipeline that was seen running and is gone, which is what <see cref="_open"/> is
    /// held for: a key that has never been seen running is one whose answer is still on its way, and asking
    /// again would only duplicate it.
    /// </summary>
    private void Receive(WatchKey? want)
    {
        // The decode this card no longer wants goes first, so a route switch or a leg that moved cannot leave
        // two open.
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
            // Running is the state this asked for, and there is nothing to do about it.
            if (Decoding(want) is not null)
            {
                _open = true;
                return;
            }

            // Asked for and not running: either the answer has not come back yet, or the leg answered with a
            // refusal.
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

    /// <summary>Whether a key names the same decode as another, and false where either is absent.</summary>
    private static bool Names(WatchKey? key, WatchKey? other)
        => key is not null && other is not null
            && key.StreamName == other.StreamName && key.Transport == other.Transport;

    /// <summary>
    /// Asks the relay for a decode of this machine's own stream, and holds the refusal where there is one.
    ///
    /// Nothing is written about what the decode became: the answer carries no state and what is running
    /// arrives on the event stream, which is the one path into the display.
    /// What lands here is only whether there is a sentence to show and that the next pass may ask again.
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
            // The backend's own sentence: a leg that cannot carry this stream's format names the format and
            // the protocols that would have carried it, which is the whole of what makes the refusal
            // actionable.
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
                // The route was toggled while this was in flight, so the card has already let go of the key -
                // and the stop that went with it may have reached the backend before this start did, which
                // would leave a decode open that nothing wants.
                // Letting go a second time is what closes it, and costs nothing where the first one already
                // did: a stop naming a decode that is not running is a success.
                Release(key);
            }

            Apply();
        });
    }

    /// <summary>
    /// Lets go of one decode, and closes it unless the viewer's grid is drawing the same pair.
    ///
    /// <b>A decode is one pipeline whoever asked for it</b>, keyed by the stream and the leg, so a stop
    /// issued here takes the picture out of every window drawing it.
    /// Where the grid holds a tile on the same pair, this card lets go of the ask and leaves the pipeline to
    /// the window that still wants it; the grid's own stop is what closes it then (<see cref="SetGridLeg"/>).
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
            // A decode this card can no longer reach is one it can no longer close, and the card has already
            // stopped drawing it.
            // The backend's own roster is what answers for it from here, which is the same place a shell that
            // crashed leaves it.
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <summary>
    /// Converges on the picture that is running: makes the tile when there is one to draw and drops it when
    /// there is not.
    ///
    /// A tile is kept rather than rebuilt while it names the same picture, because rebuilding one would
    /// restart a subscription that is already drawing it.
    /// The comparison is the whole source and not the stream name alone, so an end-to-end route whose leg
    /// moved gets the tile rebuilt on the leg it is now receiving over.
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

        // Nothing to rearrange: this card draws one tile with no grid around it, so focus, pop-out and
        // fullscreen have no meaning here and the intents go nowhere.
        // The preview's own template offers none of them either - it is the tile that is reused, not the
        // viewer's menu.
        Draw(new TileViewModel(source, _backend, _dispatch, _ => { }));
    }

    /// <summary>
    /// Puts one tile on the card or takes it off, and moves the subscription to its reports with it.
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
            // A tile reports what it drew, which no state the backend owns can carry - a backend cannot see
            // that a compositor was too slow to take a frame.
            // The pass it asks for is this card's own, so the picture and the figures over it are still
            // written by one render function.
            tile.Changed += Apply;
        }

        Tile = tile;
    }

    /// <summary>
    /// Why the card is dark, in the order a reader can act on.
    ///
    /// A refusal comes first, because it is the one sentence that names something to change: a leg that
    /// cannot carry this stream's format is answered by choosing another, and every state under it is
    /// answered by waiting.
    /// A tile answers for itself next, because the three reasons a tile has nothing to draw are the tile's
    /// own and are already written there.
    /// What is left is this card's, and the last of them is the chosen route's - the two routes are dark for
    /// different reasons and neither is folded into the other.
    /// </summary>
    private string PlaceholderFor()
    {
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
