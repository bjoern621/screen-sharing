using System.Runtime.CompilerServices;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Broadcast.Preview.Model;
using ScreenShare.App.Features.Broadcast.Preview.ViewModel;
using ScreenShare.App.Features.Viewer.Tile.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Two pictures of the stream this machine is sending, and which of the two pays the relay.
/// The local route is a copy the publish child writes to a loopback port, so no decode is opened, none is closed
/// and no reader slot is taken, which keeps the viewer figures beside the card about viewers.
/// The end-to-end route is a decode of this machine's own stream off the relay, asserted against the calls the
/// seam received rather than against the picture, a receive effect on the wrong route being invisible on screen.
/// The lifecycle fails invisibly on the same terms: a converge that rebuilds the tile per pass restarts a frame
/// subscription a second, and one that lets go of a decode without closing it leaves a reader on the relay for
/// the life of the window.
/// The off segment belongs to that lifecycle rather than to blanking the tile, being what gives the relay slot
/// back.
/// </summary>
public sealed class BroadcastPreviewTests
{
    private const string Stream = "desk";

    /// <summary>
    /// A backend whose running state a test writes: what is publishing, and whether it is being previewed.
    /// Every receive effect and every frame subscription asked for is recorded, and that is what the converge is
    /// judged on.
    /// The rest forwards to <see cref="SeededBackend"/>: two sets of answers would be two fixtures to keep in
    /// step.
    /// </summary>
    private sealed class PreviewBackend : IBackend
    {
        private readonly SeededBackend _seed = new("windows");

        public event Action? Changed
        {
            add { }
            remove { }
        }

        /// <summary>What is publishing. Nothing until a test writes one, which the absent <c>Live</c> says.</summary>
        public PublishState Publish { get; set; } = new();

        /// <summary>
        /// Every relay decode a start or a stop named, oldest first.
        /// The local route adds to neither; the end-to-end route is judged on which pairs it opened and
        /// closed.
        /// </summary>
        public List<StreamRef> Started { get; } = [];

        public List<StreamRef> Stopped { get; } = [];

        /// <summary>
        /// Why the next start is refused, empty while none is.
        /// Stands for a leg that cannot carry this stream's format, the refusal the end-to-end route meets.
        /// </summary>
        public string StartRefusal { get; set; } = "";

        public int PreviewSubscriptions { get; private set; }

        public int RelaySubscriptions { get; private set; }

        /// <summary>
        /// Opens one decode and records it.
        /// The seed holds the open set, so <see cref="ReceivingAsync"/> answers with this call's effect rather
        /// than with a second list kept here.
        /// </summary>
        public Task StartReceiveAsync(
            string streamName, string transport, bool toneMap = false, CancellationToken cancellation = default)
        {
            Started.Add(new StreamRef { StreamName = streamName, Transport = transport });

            return StartRefusal.Length > 0
                ? Task.FromException(new BackendUnavailableException(StartRefusal))
                : _seed.StartReceiveAsync(streamName, transport, toneMap, cancellation);
        }

        public Task StopReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        {
            Stopped.Add(new StreamRef { StreamName = streamName, Transport = transport });
            return _seed.StopReceiveAsync(streamName, transport, cancellation);
        }

        // No GPU and no pipeline behind a fixture, so the ask is recorded and then refused.
        // A made-up stream of handles would name GPU memory that does not exist.
        public Task<FrameChannel> OpenPreviewFramesAsync(CancellationToken cancellation = default)
        {
            PreviewSubscriptions++;
            throw new BackendUnavailableException("this fixture lends no frames");
        }

        public Task<FrameChannel> OpenFramesAsync(string streamName, string transport, CancellationToken cancellation = default)
        {
            RelaySubscriptions++;
            throw new BackendUnavailableException("this fixture lends no frames");
        }

        // Screen pictures belong to the wizard's picker rather than to the broadcast card.
        // Present to satisfy the seam.
        public Task StartMonitorPreviewAsync(int monitor, CancellationToken cancellation = default)
            => Task.CompletedTask;

        public Task StopMonitorPreviewAsync(int monitor, CancellationToken cancellation = default)
            => Task.CompletedTask;

        public Task<FrameChannel> OpenMonitorFramesAsync(int monitor, CancellationToken cancellation = default)
            => throw new BackendUnavailableException("this fixture lends no frames");

        public Task<IReadOnlyList<PreviewedMonitor>> PreviewedMonitorsAsync(CancellationToken cancellation = default)
            => Task.FromResult<IReadOnlyList<PreviewedMonitor>>([]);

        /// <summary>
        /// The preview carries no sound, so neither effect has anything to do.
        /// Answered rather than refused: a refusal is a state a test could read as the card's own.
        /// </summary>
        public Task SetReceiveAudioAsync(
            string streamName, string transport, double volume, bool muted, CancellationToken cancellation = default)
            => Task.CompletedTask;

        public async IAsyncEnumerable<AudioLevels> SubscribeAudioLevelsAsync(
            [EnumeratorCancellation] CancellationToken cancellation = default)
        {
            await Task.CompletedTask.ConfigureAwait(false);
            yield break;
        }

        public async IAsyncEnumerable<PointerPosition> SubscribePointerAsync(
            [EnumeratorCancellation] CancellationToken cancellation = default)
        {
            await Task.CompletedTask.ConfigureAwait(false);
            yield break;
        }

        public Task<PublishState> PublishStateAsync(CancellationToken cancellation = default)
            => Task.FromResult(Publish);

        public Task<RelayStatus> RelayStatusAsync(CancellationToken cancellation = default)
            => _seed.RelayStatusAsync(cancellation);

        public Task<IReadOnlyList<ReceiveStream>> ReceivingAsync(CancellationToken cancellation = default)
            => _seed.ReceivingAsync(cancellation);

        public Task<Form> ResolveFormAsync(Settings draft, CancellationToken cancellation = default)
            => _seed.ResolveFormAsync(draft, cancellation);

        public Task<Catalog> CatalogAsync(CancellationToken cancellation = default)
            => _seed.CatalogAsync(cancellation);

        public Task<Settings> SettingsAsync(CancellationToken cancellation = default)
            => _seed.SettingsAsync(cancellation);

        public Task<IReadOnlyList<StreamRef>> WatchingAsync(CancellationToken cancellation = default)
            => _seed.WatchingAsync(cancellation);

        public Task StartPublishAsync(Settings settings, CancellationToken cancellation = default)
            => _seed.StartPublishAsync(settings, cancellation);

        public Task SaveSettingsAsync(Settings settings, CancellationToken cancellation = default)
            => _seed.SaveSettingsAsync(settings, cancellation);

        public Task ApplyToStreamAsync(Settings settings, CancellationToken cancellation = default)
            => _seed.ApplyToStreamAsync(settings, cancellation);

        public Task StopPublishAsync(CancellationToken cancellation = default)
            => _seed.StopPublishAsync(cancellation);

        public Task<double> MeasureUplinkAsync(CancellationToken cancellation = default)
            => _seed.MeasureUplinkAsync(cancellation);

    public Task<IReadOnlyList<RelayLeg>> CheckRelayAsync(Settings settings, CancellationToken cancellation = default)
            => _seed.CheckRelayAsync(settings, cancellation);



        public Task<(string Key, string Id)> CreateGroupAsync(RelaySettings relay, CancellationToken cancellation = default)
            => _seed.CreateGroupAsync(relay, cancellation);

        public Task<MembersState> MembersAsync(CancellationToken cancellation = default)
            => _seed.MembersAsync(cancellation);

        public Task<TestStreamState> TestStreamsAsync(CancellationToken cancellation = default)
            => _seed.TestStreamsAsync(cancellation);

        public Task JoinGroupAsync(CancellationToken cancellation = default)
            => _seed.JoinGroupAsync(cancellation);

        public Task LeaveGroupAsync(CancellationToken cancellation = default)
            => _seed.LeaveGroupAsync(cancellation);

        public Task<PresetStore> PresetsAsync(CancellationToken cancellation = default)
            => _seed.PresetsAsync(cancellation);

        public Task SavePresetAsync(string name, PublishSettings settings, CancellationToken cancellation = default)
            => _seed.SavePresetAsync(name, settings, cancellation);

        public Task DeletePresetAsync(string name, CancellationToken cancellation = default)
            => _seed.DeletePresetAsync(name, cancellation);

        public Task StartWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
            => _seed.StartWatchAsync(streamName, transport, cancellation);

        public Task StopWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
            => _seed.StopWatchAsync(streamName, transport, cancellation);

        public Task OpenInBrowserAsync(string streamName, string transport, CancellationToken cancellation = default)
            => _seed.OpenInBrowserAsync(streamName, transport, cancellation);

        public Task OpenLogAsync(string path, CancellationToken cancellation = default)
            => _seed.OpenLogAsync(path, cancellation);

        public Task OpenLogsFolderAsync(CancellationToken cancellation = default)
            => _seed.OpenLogsFolderAsync(cancellation);

        /// <summary>
        /// A stream that ends at once, so nothing arrives after the session's first read.
        /// Running state is written on this fixture and read again.
        /// </summary>
        public async IAsyncEnumerable<Event> SubscribeAsync(
            [EnumeratorCancellation] CancellationToken cancellation = default)
        {
            await Task.CompletedTask.ConfigureAwait(false);
            yield break;
        }
    }

    /// <summary>A stream in force under this suite's name, with a local preview behind it.</summary>
    private static PublishState Live(bool previewed = true)
    {
        var live = new PublishState.Types.Live { Publish = new PublishSettings { Name = Stream } };
        if (previewed)
        {
            live.Preview = new PublishState.Types.Preview { Port = 45678 };
        }

        return new PublishState { Live = live };
    }

    /// <summary>A previewed publish whose preview pipeline has, or has not, put out a frame.</summary>
    private static PublishState Decoding(bool live)
    {
        var state = Live();
        state.Live.Preview.Live = live;
        return state;
    }

    /// <summary>Leg the fixture's stored settings name for a tile, and so the end-to-end route's.</summary>
    private const string Leg = "srt";

    private static (PreviewViewModel Preview, Session Session) Card(PreviewBackend backend)
        => Card(backend, PreviewRoute.Local);

    /// <summary>
    /// A card on one route, over a session that has read the fixture once.
    /// The session is started rather than written to: every state a screen reads is one the backend answered
    /// with.
    /// </summary>
    private static (PreviewViewModel Preview, Session Session) Card(PreviewBackend backend, PreviewRoute route)
    {
        var session = Read(backend);
        var preview = new PreviewViewModel(backend, Settings(backend, session), session, static action => action());

        Choose(preview, route);
        Settle(preview, session);
        return (preview, session);
    }

    /// <summary>
    /// Lets the card see what its own effects did.
    /// What is decoding moves when a decode opens or closes, and the app learns it off the event stream, which
    /// this fixture ends at once, so a second read stands in for it.
    /// </summary>
    private static void Settle(PreviewViewModel preview, Session session)
    {
        session.Start();
        preview.Apply();
    }

    /// <summary>Picks a route as the toggle does, by handing over the segment that stands for it.</summary>
    private static void Choose(PreviewViewModel preview, PreviewRoute route)
        => preview.SelectedRoute = preview.Routes.Single(tab => tab.Value == route);

    /// <summary>
    /// A session that has read the fixture.
    /// Every answer is completed and the dispatcher is straight through, so the read has landed on return.
    /// </summary>
    private static Session Read(PreviewBackend backend)
    {
        var session = new Session(backend, static action => action());
        session.Start();
        return session;
    }

    /// <summary>
    /// The backend's stored settings, read as the window reads them.
    /// One value out of them is the card's, the end-to-end route's leg, and the seeded answer names
    /// <see cref="Leg"/>.
    /// </summary>
    private static FormSession Settings(PreviewBackend backend, Session session)
        => new(backend, session, static action => action());

    [Fact]
    public void TheConvergeDrawsThePreviewTheBackendIsRunning()
    {
        var backend = new PreviewBackend { Publish = Live() };

        var (preview, _) = Card(backend);

        Assert.NotNull(preview.Tile);
        Assert.True(preview.HasTile);
        Assert.Equal(Stream, preview.Tile.Name);
        Assert.Equal(TileSourceKind.PublishPreview, preview.Tile.Source.Kind);
        Assert.Equal("", preview.Tile.Transport);
    }

    [Fact]
    public void ASecondPassOverUnchangedInputChangesNothing()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Card(backend);

        var tile = preview.Tile;
        Assert.NotNull(tile);

        preview.Apply();
        preview.Apply();
        Choose(preview, PreviewRoute.Local);

        // The same tile, not an equal one: a tile is a running frame subscription, so a rebuilt one is a
        // restarted subscription however alike the two look.
        Assert.Same(tile, preview.Tile);
        Assert.Empty(backend.Started);
        Assert.Empty(backend.Stopped);
    }

    /// <summary>
    /// The picture a publisher wants is the one being sent, so a card that had to be started first would read
    /// as broken.
    /// </summary>
    [Fact]
    public void TheCardOpensOnARouteAndDrawing()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var session = Read(backend);

        var preview = new PreviewViewModel(backend, Settings(backend, session), session, static action => action());

        Assert.Equal(PreviewRoute.Local, preview.SelectedRoute.Value);
        Assert.NotNull(preview.Tile);
        Assert.True(preview.HasTile);
    }

    /// <summary>
    /// The off sentence outranks every state of the machine: the card is dark because it was asked to be, and a
    /// sentence about the stream would send a reader after a problem nobody has.
    /// </summary>
    [Fact]
    public void TheOffSegmentEndsTheSubscription()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Card(backend);
        Assert.NotNull(preview.Tile);

        Choose(preview, PreviewRoute.Off);

        Assert.Equal(PreviewRoute.Off, preview.SelectedRoute.Value);
        Assert.Null(preview.Tile);
        Assert.False(preview.HasTile);
        Assert.Equal(Cards.PreviewOff, preview.Placeholder);
    }

    /// <summary>Repeating the write is what makes it safe to call from a render pass.</summary>
    [Fact]
    public void GoingOffTwiceIsTheSameAsGoingOffOnce()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Card(backend, PreviewRoute.EndToEnd);

        Choose(preview, PreviewRoute.Off);
        Choose(preview, PreviewRoute.Off);
        preview.Apply();

        Assert.Single(backend.Stopped);
        Assert.Null(preview.Tile);
    }

    [Fact]
    public void GoingOffAirEndsTheSubscription()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, session) = Card(backend);
        Assert.NotNull(preview.Tile);

        // The session learns the stream ended the way it learns everything, by reading again.
        backend.Publish = new PublishState();
        session.Start();
        preview.Apply();

        Assert.Null(preview.Tile);
        Assert.False(preview.HasTile);
        Assert.Equal(Cards.PreviewNotPublishing, preview.Placeholder);
    }

    [Fact]
    public void ComingBackOnARouteDrawsAgain()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Card(backend);

        Choose(preview, PreviewRoute.Off);
        Choose(preview, PreviewRoute.Local);

        Assert.NotNull(preview.Tile);
        Assert.Equal(TileSourceKind.PublishPreview, preview.Tile.Source.Kind);
    }

    [Fact]
    public void NothingPublishingIsItsOwnSentence()
    {
        var backend = new PreviewBackend();

        var (preview, _) = Card(backend);

        Assert.Null(preview.Tile);
        Assert.True(preview.HasPlaceholder);
        Assert.Equal(Cards.PreviewNotPublishing, preview.Placeholder);
    }

    /// <summary>
    /// A real state, reached by a format with no local carriage or a preview pipeline that would not start.
    /// The publish is untouched either way, what the leg being a copy buys.
    /// </summary>
    [Fact]
    public void APublishWithNoPreviewIsItsOwnSentence()
    {
        var backend = new PreviewBackend { Publish = Live(previewed: false) };

        var (preview, _) = Card(backend);

        Assert.Null(preview.Tile);
        Assert.Equal(Cards.PreviewNotPreviewed, preview.Placeholder);
    }

    [Fact]
    public void ThePlaceholderStatesAreDistinct()
    {
        var sentences = new[]
        {
            Cards.PreviewNotPublishing,
            Cards.PreviewNotPreviewed,
            Cards.PreviewNoWatchLeg,
            Cards.PreviewOpening,
            Cards.PreviewOff,
            "Nothing is decoding this stream.",
            "Connecting.",
        };

        Assert.Equal(sentences.Length, sentences.Distinct().Count());
        Assert.All(sentences, sentence => Assert.NotEqual("", sentence));
    }

    /// <summary>
    /// The preview is part of the publish, so its pipeline's state travels on the publish state rather than
    /// through a decode list.
    /// </summary>
    [Fact]
    public void ATileWithNoFrameYetSaysWhichStateItIsIn()
    {
        var backend = new PreviewBackend { Publish = Decoding(live: false) };
        var (preview, session) = Card(backend);

        Assert.Equal("Connecting.", preview.Placeholder);

        backend.Publish = Decoding(live: true);
        session.Start();
        preview.Apply();

        Assert.Equal("", preview.Placeholder);
        Assert.False(preview.HasPlaceholder);
    }

    /// <summary>
    /// The one thing a reader must not discover the hard way: the picture is what is being sent, and says
    /// nothing about what viewers receive.
    /// </summary>
    [Fact]
    public void TheCardStatesWhatThePictureIsAndIsNot()
    {
        var backend = new PreviewBackend { Publish = Live() };

        var (preview, _) = Card(backend);

        Assert.Equal(Cards.PreviewLocalCost, preview.Cost);
        Assert.Contains("never reaches the relay", preview.Cost);
        Assert.Contains("nothing about what viewers receive", preview.Cost);
    }

    // --- The route toggle -----------------------------------------------------------
    //
    // The half that is invisible on screen: which route asks the relay for a decode, which asks it for
    // nothing, and that a switch between them leaves exactly one open.

    [Fact]
    public void TheToggleOffersEveryRouteAndOpensOnTheLocalOne()
    {
        var backend = new PreviewBackend { Publish = Live() };

        var (preview, _) = Card(backend);

        Assert.Equal(PreviewRoutes.All, preview.Routes.Select(tab => tab.Value));
        Assert.Equal(PreviewRoute.Local, preview.SelectedRoute.Value);
        Assert.All(preview.Routes, tab => Assert.NotEqual("", tab.Label));
        Assert.NotEqual("", preview.RouteChoice);
    }

    /// <summary>
    /// A viewer of this machine's own stream opens a decode as the grid does: the publishing stream, over the
    /// leg the stored settings name for a tile.
    /// </summary>
    [Fact]
    public void TheEndToEndRouteReceivesThisMachinesOwnStream()
    {
        var backend = new PreviewBackend { Publish = Live() };

        var (preview, _) = Card(backend, PreviewRoute.EndToEnd);

        var opened = Assert.Single(backend.Started);
        Assert.Equal(Stream, opened.StreamName);
        Assert.Equal(Leg, opened.Transport);

        Assert.NotNull(preview.Tile);
        Assert.True(preview.Tile.Source.IsRelay);
        Assert.Equal(Leg, preview.Tile.Transport);
        Assert.True(preview.HasLeg);
    }

    /// <summary>
    /// The two claims about the relay are opposite, so one sentence for both would be false under one of them.
    /// </summary>
    [Fact]
    public void EachSegmentStatesItsOwnCost()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Card(backend);

        Assert.Equal(Cards.PreviewLocalCost, preview.Cost);

        Choose(preview, PreviewRoute.EndToEnd);

        Assert.Equal(Cards.PreviewEndToEndCost, preview.Cost);
        Assert.Contains("what a viewer receives", preview.Cost);

        Choose(preview, PreviewRoute.Off);

        Assert.Equal(Cards.PreviewOffCost, preview.Cost);

        var sentences = new[] { Cards.PreviewOffCost, Cards.PreviewLocalCost, Cards.PreviewEndToEndCost };
        Assert.Equal(sentences.Length, sentences.Distinct().Count());
    }

    /// <summary>
    /// A route that let go of its ask without closing would leave a reader slot on the relay for the life of
    /// the window.
    /// </summary>
    [Fact]
    public void SwitchingRoutesLeavesExactlyOneDecodeOpen()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, session) = Card(backend, PreviewRoute.EndToEnd);

        Assert.Single(backend.Started);
        Assert.Empty(backend.Stopped);

        Choose(preview, PreviewRoute.Local);

        var closed = Assert.Single(backend.Stopped);
        Assert.Equal(Stream, closed.StreamName);
        Assert.Equal(Leg, closed.Transport);
        Assert.Equal(TileSourceKind.PublishPreview, preview.Tile?.Source.Kind);

        Settle(preview, session);
        Choose(preview, PreviewRoute.EndToEnd);

        Assert.Equal(2, backend.Started.Count);
        Assert.Single(backend.Stopped);
    }

    /// <summary>What the loopback leg exists for: the relay serves no reader for that picture and counts none.</summary>
    [Fact]
    public async Task TheLocalRouteAsksTheRelayForNothing()
    {
        var backend = new PreviewBackend { Publish = Live() };

        var (preview, _) = Card(backend);
        Assert.NotNull(preview.Tile);

        await Assert.ThrowsAsync<BackendUnavailableException>(
            () => preview.Tile.OpenAsync(CancellationToken.None));

        Assert.Equal(1, backend.PreviewSubscriptions);
        Assert.Equal(0, backend.RelaySubscriptions);
        Assert.Empty(backend.Started);
        Assert.Empty(backend.Stopped);
    }

    /// <summary>
    /// The pass runs on every event the session reports, so a converge that re-asked would spend a round trip
    /// a second on a decode that is open.
    /// </summary>
    [Fact]
    public void ASecondPassOnTheEndToEndRouteAsksForNothing()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Card(backend, PreviewRoute.EndToEnd);

        var tile = preview.Tile;
        Assert.NotNull(tile);

        preview.Apply();
        preview.Apply();
        Choose(preview, PreviewRoute.EndToEnd);

        Assert.Single(backend.Started);
        Assert.Empty(backend.Stopped);
        Assert.Same(tile, preview.Tile);
    }

    /// <summary>
    /// The reader slot is what this route costs the relay, so a stop that closed only the subscription would
    /// give back the picture and none of what it was paying for.
    /// </summary>
    [Fact]
    public void GoingOffClosesTheEndToEndDecode()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Card(backend, PreviewRoute.EndToEnd);

        Choose(preview, PreviewRoute.Off);

        Assert.Single(backend.Stopped);
        Assert.Null(preview.Tile);
    }

    /// <summary>
    /// One pipeline serves both windows, so a stop from this card would take the picture out of a tile the
    /// reader is watching on the other screen.
    /// </summary>
    [Fact]
    public void ADecodeTheGridIsDrawingIsLeftOpen()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Card(backend, PreviewRoute.EndToEnd);

        preview.SetGridLeg(stream => stream == Stream ? Leg : "");
        Choose(preview, PreviewRoute.Off);

        Assert.Single(backend.Started);
        Assert.Empty(backend.Stopped);
    }

    /// <summary>
    /// A leg that cannot carry the stream's format names the format and the protocols that would have carried
    /// it, which is nothing this side could compose.
    /// </summary>
    [Fact]
    public void ARefusedDecodeSaysWhyInTheBackendsWords()
    {
        const string refusal = "srt does not carry av1: rtsp and webrtc do.";
        var backend = new PreviewBackend { Publish = Live(), StartRefusal = refusal };

        var (preview, _) = Card(backend, PreviewRoute.EndToEnd);

        Assert.Equal(refusal, preview.Placeholder);
        Assert.True(preview.HasPlaceholder);
        Assert.Null(preview.Tile);

        // A refusal is a fact about the key rather than a moment, so re-asking per pass would be a round trip
        // a second against a leg that has answered.
        preview.Apply();
        preview.Apply();
        Assert.Single(backend.Started);
    }

    /// <summary>
    /// Held across the switch, the sentence would stand under the other route's picture describing a leg that
    /// route never uses.
    /// </summary>
    [Fact]
    public void LeavingARefusedRouteClearsItsSentence()
    {
        var backend = new PreviewBackend
        {
            Publish = Decoding(live: true),
            StartRefusal = "srt does not carry av1.",
        };
        var (preview, _) = Card(backend, PreviewRoute.EndToEnd);
        Assert.Equal(backend.StartRefusal, preview.Placeholder);

        Choose(preview, PreviewRoute.Local);

        Assert.Equal("", preview.Placeholder);
        Assert.False(preview.HasPlaceholder);
        Assert.Equal(TileSourceKind.PublishPreview, preview.Tile?.Source.Kind);
    }
}
