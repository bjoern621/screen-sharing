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
/// The program preview: what is being sent, and what it costs to show it.
///
/// <b>What it draws never leaves this machine.</b> The publish child copies its
/// already-encoded video to a loopback port beside the sink that feeds the relay, and the
/// backend decodes what arrives there. The relay is not a party to it: it serves no reader
/// for this picture, counts none, and measures none
/// (<c>docs/viewer-architecture.md</c>, "What the broadcast preview draws").
///
/// <b>It shows what is being sent, not what a viewer receives.</b> That is the one thing a
/// reader must not have to discover: the picture is taken before the relay, so nothing
/// downstream of the encoder is visible in it - a congested uplink, a relay dropping packets
/// and a viewer on a bad link all leave this card looking perfect. What answers those is the
/// viewer table and the round-trip plot beside it, which measure the readers the relay really
/// has. The card says so in its own words rather than leaving it to be found out
/// (<see cref="Cost"/>).
///
/// <b>It opens nothing and closes nothing.</b> The preview pipeline goes up with the publish
/// child and down with it, so there is no effect here to call and nothing here to converge on
/// beyond the subscription: <c>PublishState.Live.preview</c> is what says whether there is a
/// picture to ask for, read through on every pass like every other state. That is also why
/// this card needs no leg and no relay path - it borrows no <c>WatchKey</c>, because the
/// frames crossed no protocol.
///
/// <b>The lifecycle is a converge and not a sequence.</b> <see cref="Apply"/> states the world
/// it wants - a tile subscribed to the running preview, or nothing - and <see cref="Converge"/>
/// decides whether the tile has to be made or dropped. A second pass over unchanged input
/// makes nothing, drops nothing and subscribes to nothing twice.
/// </summary>
public sealed class PreviewViewModel : Observable
{
    private readonly IBackend _backend;
    private readonly Session _session;
    private readonly Action<Action> _dispatch;

    /// <summary>
    /// Whether this card is in a visual tree. It is the one fact the view knows and the view
    /// model cannot read: the shell renders every destination on every pass, so without it a
    /// reader who never opened this screen would still be paying for the frame traffic and the
    /// GPU copies of a picture nobody is looking at. Written by <see cref="SetShowing"/> and by
    /// nothing else.
    /// </summary>
    private bool _showing;

    /// <summary>
    /// The tile this card draws, and null while it wants none.
    ///
    /// It is <see cref="TileViewModel"/> and not a second frame consumer written here. A tile
    /// is a source, the figures over it and the subscription the control behind it reads, and
    /// this card is one of those - a second implementation would be a second answer to what a
    /// dropped frame is and where a lent handle goes back.
    /// </summary>
    private TileViewModel? _tile;

    /// <param name="dispatch">
    /// Hands work to the UI loop. The tile's own reports land on whichever thread the transport
    /// completed on, and everything this writes is read by a binding that only tolerates being
    /// written from one.
    /// </param>
    public PreviewViewModel(IBackend backend, Session session, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a preview subscribes to the backend's frames");
        Assert.NotNull(session, "a preview renders the session's running state");
        Assert.NotNull(dispatch, "a preview needs a UI loop to marshal a report back to");

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
    /// distinct on purpose: nothing is publishing, the publish is running without a preview,
    /// and - once there is a tile - the tile's own three (the pipeline is not reported yet, it
    /// is up with no frame out of it, or this window could not draw what it was handed).
    /// </summary>
    public string Placeholder { get => _placeholder; private set => Set(ref _placeholder, value); }

    public bool HasPlaceholder { get => _hasPlaceholder; private set => Set(ref _hasPlaceholder, value); }

    /// <summary>
    /// What this picture is and is not: a local copy of what is being sent, costing one decode
    /// on this machine and showing nothing about what the relay does with it afterwards. Stated
    /// on the card rather than in a comment, because a preview that looks perfect while viewers
    /// suffer is exactly the misreading it exists to prevent.
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
    /// Safe to run twice: the converge keeps the tile it already has for the preview it is
    /// already drawing, and every property setter compares before it notifies.
    /// </summary>
    public void Apply()
    {
        var reading = Snapshot;
        var preview = Running();

        Converge(preview);

        // The tile is rendered from the state the backend reports about the pipeline behind
        // it, read through on every pass. A tile whose pipeline is not in the state draws its
        // own reason for that rather than disappearing.
        _tile?.Apply(TilePipeline.Of(preview));

        IsOnAir = reading.IsLive;
        Encoded = $"encoded {Figure.Of(reading.Fps, "0.0")} fps";
        Quality = $"cq {Figure.Of(reading.Cq)}";
        Cost = Cards.PreviewCost;

        // After the tile's own pass, because a tile that is drawing has no sentence and one
        // that is not has written it by now.
        Placeholder = PlaceholderFor();
        HasPlaceholder = Placeholder.Length > 0;
        HasTile = _tile is not null;

        Assert.That(HasPlaceholder == (Placeholder.Length > 0), "a placeholder and its sentence agree", HasPlaceholder);
        Assert.That(HasTile == (Tile is not null), "a tile and the fact that there is one agree", HasTile);
        Assert.That(_tile is null || preview is not null, "a preview tile draws a preview the backend is running");
        Assert.That(Cost.Length > 0, "a preview states what it is showing and what it is not");
    }

    /// <summary>
    /// The preview the backend is running, and null for "nothing to draw".
    ///
    /// Two facts have to hold and each is read through rather than remembered: the card is on
    /// screen, and a publish is in force with a preview behind it. The second is one field -
    /// the backend brings the preview up with the publish child, so its presence is the whole
    /// answer and there is nothing here to ask for or wait on.
    /// </summary>
    private PublishState.Types.Preview? Running()
        => _showing ? _session.Publish?.Live?.Preview : null;

    /// <summary>
    /// Converges on the preview that is running: makes the tile when there is one to draw and
    /// drops it when there is not.
    ///
    /// <b>It calls nothing.</b> There is no effect to open a preview and none to close one, so
    /// the whole of the converge is the subscription - which is the control's, opened when the
    /// tile is bound and cancelled when it is not. A tile is kept rather than rebuilt while the
    /// preview goes on running, because rebuilding one would restart a subscription that is
    /// already drawing the same pipeline.
    /// </summary>
    private void Converge(PublishState.Types.Preview? preview)
    {
        if (preview is null)
        {
            Draw(null);
            return;
        }

        // The stream name is what the tile's heading prints; the subscription names nothing,
        // because the backend has one publish to preview. A publish that is live always names
        // itself, and the empty string is a state no tile is made for rather than one drawn
        // with a blank heading.
        var stream = _session.Publish?.Live?.Publish?.Name ?? "";
        if (stream.Length == 0)
        {
            Draw(null);
            return;
        }

        if (_tile is not null && _tile.Name == stream)
        {
            return;
        }

        Draw(new TileViewModel(TileSource.Preview(stream), _backend, _dispatch));
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
    /// Why the card is dark, in the order the states happen in.
    ///
    /// A tile answers for itself once there is one, because the three reasons a tile has
    /// nothing to draw are the tile's own and are already written there. Before that the states
    /// are this card's: nothing is publishing, or a publish is running that the backend is not
    /// previewing. They are different things to do about it, which is why neither is folded
    /// into the other.
    /// </summary>
    private string PlaceholderFor()
    {
        if (_tile is not null)
        {
            return _tile.Notice;
        }

        if (_session.Publish?.Live is null)
        {
            return Cards.PreviewNotPublishing;
        }

        return Cards.PreviewNotPreviewed;
    }
}
