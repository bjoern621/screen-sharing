using System.Runtime.CompilerServices;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Insights.Preview.Model;
using ScreenShare.App.Features.Insights.Preview.ViewModel;
using ScreenShare.App.Features.Viewer.Tile.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Two pictures of the stream this machine is sending, and which of the two pays the relay.
/// Local route: a copy the publish child writes to a loopback port, so no decode opens or closes,
/// no reader slot is taken, and the viewer figures beside the card stay about viewers.
/// End-to-end route: a decode of this machine's own stream off the relay, judged on the calls the boundary
/// received rather than on the picture, a receive effect on the wrong route being invisible on screen.
/// The lifecycle fails invisibly too: a tile rebuilt per pass restarts a frame subscription a second,
/// and a decode let go of without a close leaves a reader on the relay for the life of the window.
/// Off gives that reader slot back, so it belongs to the lifecycle rather than to blanking the tile.
/// </summary>
public sealed class InsightsPreviewTests
{
    private const string Stream = "desk";

    /// <summary>
    /// Running state a test writes: what is publishing, and whether it is being previewed.
    /// Every receive effect and every frame subscription asked for is recorded, and the converge is judged on that.
    /// The rest forwards to <see cref="SeededBackend"/>, two answer sets being two fixtures to keep in step.
    /// </summary>
    private sealed class PreviewBackend : IBackend
    {
        private readonly SeededBackend _seed = new("windows");

        public Task<string> VersionAsync(CancellationToken cancellation = default) => _seed.VersionAsync(cancellation);

        public event Action? Changed
        {
            add { }
            remove { }
        }

        /// <summary>What is publishing. Absent <c>Live</c> until a test writes one.</summary>
        public PublishState Publish { get; set; } = new();

        /// <summary>
        /// Every relay decode a start or a stop named, oldest first.
        /// The local route adds to neither.
        /// </summary>
        public List<StreamRef> Started { get; } = [];

        public List<StreamRef> Stopped { get; } = [];

        /// <summary>
        /// Why the next start is refused, empty while none is.
        /// Stands for a leg that cannot carry this stream's format.
        /// </summary>
        public string StartRefusal { get; set; } = "";

        /// <summary>
        /// Route the stored settings name, empty for the one the seed itself holds.
        /// What a card left on a route reads back at the next start.
        /// </summary>
        public string StoredRoute { get; set; } = "";

        /// <summary>Settings the card asked to keep, oldest first.</summary>
        public IReadOnlyList<Settings> Saved => _seed.Saved;

        public int PreviewSubscriptions { get; private set; }

        public int RelaySubscriptions { get; private set; }

        /// <summary>
        /// The seed holds the open set, so <see cref="ReceivingAsync"/> answers with this call's effect,
        /// not a second list kept here.
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
        // Made-up frame handles would name GPU memory that does not exist.
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

        // Screen pictures belong to the wizard's picker rather than to the insights card.
        public Task StartMonitorPreviewAsync(int monitor, CancellationToken cancellation = default)
            => Task.CompletedTask;

        public Task StopMonitorPreviewAsync(int monitor, CancellationToken cancellation = default)
            => Task.CompletedTask;

        public Task<FrameChannel> OpenMonitorFramesAsync(int monitor, CancellationToken cancellation = default)
            => throw new BackendUnavailableException("this fixture lends no frames");

        public Task<IReadOnlyList<PreviewedMonitor>> PreviewedMonitorsAsync(CancellationToken cancellation = default)
            => Task.FromResult<IReadOnlyList<PreviewedMonitor>>([]);

        /// <summary>
        /// The preview carries no sound, so nothing to do.
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
            StreamRef? stream = null,
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

        public async Task<Settings> SettingsAsync(CancellationToken cancellation = default)
        {
            var settings = await _seed.SettingsAsync(cancellation).ConfigureAwait(false);
            if (StoredRoute.Length > 0)
            {
                settings.Viewer.PreviewRoute = StoredRoute;
            }

            return settings;
        }

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

        public Task<DiscordState> DiscordAsync(CancellationToken cancellation = default)
            => _seed.DiscordAsync(cancellation);

        public Task LinkDiscordAsync(RelaySettings relay, CancellationToken cancellation = default)
            => _seed.LinkDiscordAsync(relay, cancellation);

        public Task<TestStreamState> TestStreamsAsync(CancellationToken cancellation = default)
            => _seed.TestStreamsAsync(cancellation);

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

        public Task<string> SendReportAsync(CancellationToken cancellation = default)
            => _seed.SendReportAsync(cancellation);

        public Task<UpdateState> UpdateAsync(CancellationToken cancellation = default)
            => _seed.UpdateAsync(cancellation);

        public Task CheckUpdateAsync(CancellationToken cancellation = default)
            => _seed.CheckUpdateAsync(cancellation);

        public Task InstallUpdateAsync(CancellationToken cancellation = default)
            => _seed.InstallUpdateAsync(cancellation);

        /// <summary>
        /// Ends at once, so nothing arrives after the session's first read.
        /// Running state is written on the fixture and read again instead.
        /// </summary>
        public async IAsyncEnumerable<Event> SubscribeAsync(
            [EnumeratorCancellation] CancellationToken cancellation = default)
        {
            await Task.CompletedTask.ConfigureAwait(false);
            yield break;
        }
    }

    /// <summary>Stream in force under this suite's name, with a local preview behind it.</summary>
    private static PublishState Live(bool previewed = true)
    {
        var live = new PublishState.Types.Live { Publish = new PublishSettings(), StreamName = Stream };
        if (previewed)
        {
            live.Preview = new PublishState.Types.Preview { Port = 45678 };
        }

        return new PublishState { Live = live };
    }

    /// <summary>Previewed publish whose preview pipeline has, or has not, put out a frame.</summary>
    private static PublishState Decoding(bool live)
    {
        var state = Live();
        state.Live.Preview.Live = live;
        return state;
    }

    /// <summary>Leg the fixture's stored settings name for a tile, so the end-to-end route's.</summary>
    private const string Leg = "rtsp";

    private static (PreviewViewModel Preview, Session Session) Card(PreviewBackend backend)
        => Card(backend, PreviewRoute.Local);

    /// <summary>
    /// A card on one route, over a session that has read the fixture once.
    /// The session is started rather than written to, so every state a screen reads is one the backend
    /// answered with.
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
    /// The app learns what is decoding off the event stream, which this fixture ends at once,
    /// so a second read stands in for it.
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
    /// One value out of them is the card's: the end-to-end route's leg, seeded as <see cref="Leg"/>.
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

        // The same tile, not an equal one: a tile is a running frame subscription,
        // so a rebuilt one is a restarted subscription however alike the two look.
        Assert.Same(tile, preview.Tile);
        Assert.Empty(backend.Started);
        Assert.Empty(backend.Stopped);
    }

    /// <summary>
    /// The picture a publisher wants is the one being sent, so a card that had to be started would read as broken.
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
    /// The route is a setting, so a card comes up on the picture the reader left it on, and the end-to-end one
    /// asks the relay for its decode with nobody pressing anything.
    /// </summary>
    [Fact]
    public void TheCardOpensOnTheRouteItWasLeftOn()
    {
        var backend = new PreviewBackend
        {
            Publish = Live(),
            StoredRoute = PreviewRoutes.ValueOf(PreviewRoute.EndToEnd),
        };
        var session = Read(backend);

        var preview = new PreviewViewModel(backend, Settings(backend, session), session, static action => action());
        Settle(preview, session);

        Assert.Equal(PreviewRoute.EndToEnd, preview.SelectedRoute.Value);
        var started = Assert.Single(backend.Started);
        Assert.Equal(Stream, started.StreamName);
        Assert.Equal(Leg, started.Transport);
    }

    /// <summary>
    /// The toggle has no commit beside it, so the press is the write: a route nothing kept would be a decision
    /// lost at the next start.
    /// </summary>
    [Fact]
    public void ChoosingARouteStoresIt()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Card(backend);

        Choose(preview, PreviewRoute.Off);

        Assert.Equal(PreviewRoutes.ValueOf(PreviewRoute.Off), Assert.Single(backend.Saved).Viewer.PreviewRoute);
    }

    /// <summary>Pressing the segment the card is already on names a state that holds, so nothing is written.</summary>
    [Fact]
    public void ChoosingTheRouteAlreadyDrawnStoresNothing()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Card(backend, PreviewRoute.Off);

        Choose(preview, PreviewRoute.Off);

        Assert.Single(backend.Saved);
    }

    /// <summary>
    /// The off sentence outranks every state of the machine: the card is dark because it was asked to be,
    /// and a sentence about the stream would send a reader after a problem nobody has.
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

    /// <summary>Repeating the write makes it safe to call from a render pass.</summary>
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
    /// A real state: a format with no local carriage, or a preview pipeline that would not start.
    /// The publish is untouched either way, the leg being a copy.
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
    /// The preview is part of the publish, so its pipeline's state travels on the publish state, not a decode list.
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
    // The half that is invisible on screen: which route asks the relay for a decode, which asks it for nothing,
    // and that a switch between them leaves exactly one open.

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
    /// A viewer of this machine's own stream opens a decode as the grid does:
    /// the publishing stream, over the leg the stored settings name for a tile.
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
    /// a second on a decode already open.
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
    /// The reader slot is what this route costs the relay,
    /// so a stop that closed only the subscription would give back the picture and none of the cost.
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
    /// One pipeline serves both windows,
    /// so a stop from this card would take the picture out of a tile the reader is watching on the other screen.
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
    /// A leg that cannot carry the stream's format names the format and the protocols that would have carried it.
    /// Nothing this side could compose.
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
    /// Held across the switch, the sentence would stand under the other route's picture,
    /// describing a leg that route never uses.
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

    /// <summary>
    /// A window closed to the tray draws no card, so the reader slot the end-to-end route holds is held for nobody.
    /// The route stays stored, so the picture comes back with the window.
    /// </summary>
    [Fact]
    public void HidingTheWindowClosesTheEndToEndDecodeAndShowingItAsksAgain()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, session) = Card(backend, PreviewRoute.EndToEnd);
        Assert.Single(backend.Started);

        preview.SetWindowShown(false);

        Assert.Single(backend.Stopped);
        Assert.Null(preview.Tile);
        Assert.Equal(PreviewRoute.EndToEnd, preview.SelectedRoute.Value);

        preview.SetWindowShown(true);
        Settle(preview, session);

        Assert.Equal(2, backend.Started.Count);
        Assert.NotNull(preview.Tile);
    }

    /// <summary>
    /// The local route opens no decode, so what a hidden window gives back is the frame subscription:
    /// a tile in a window that draws nothing holds the pool's slots without returning them.
    /// </summary>
    [Fact]
    public void HidingTheWindowTakesTheLocalTileDownAndShowingItBringsItBack()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Card(backend);
        Assert.NotNull(preview.Tile);

        preview.SetWindowShown(false);
        Assert.Null(preview.Tile);
        Assert.Empty(backend.Started);
        Assert.Empty(backend.Stopped);

        preview.SetWindowShown(true);
        Assert.NotNull(preview.Tile);
        Assert.Equal(TileSourceKind.PublishPreview, preview.Tile.Source.Kind);
    }

    /// <summary>A quit waits for the reader slot to be given back, so the answer is the backend's.</summary>
    [Fact]
    public void PartingClosesTheEndToEndDecode()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Card(backend, PreviewRoute.EndToEnd);

        var parted = preview.PartAsync();

        Assert.True(parted.IsCompleted);
        Assert.Single(backend.Stopped);
        Assert.Null(preview.Tile);
    }
}
