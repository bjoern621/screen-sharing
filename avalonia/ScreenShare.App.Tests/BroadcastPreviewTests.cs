using System.Runtime.CompilerServices;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Broadcast.Preview.Model;
using ScreenShare.App.Features.Broadcast.Preview.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The broadcast preview draws one of two pictures of the stream this machine is sending, and the reader
/// picks which.
///
/// <b>What these state is which route pays the relay.</b> The local route is a copy the publish child writes
/// to a loopback port, so the relay is not a party to it: no decode is opened, none is closed, and no reader
/// slot is taken - which is what keeps the viewer figures beside the card describing viewers rather than the
/// publisher watching itself.
/// The end-to-end route is the opposite by design: it is a decode of this machine's own stream off the relay,
/// and it is asserted against the calls the seam received rather than against what the card drew, because a
/// receive effect on the wrong route is invisible on screen.
///
/// <b>The rest is the lifecycle</b>, which is the other part that can go wrong invisibly: a converge that
/// rebuilds the tile on every render pass restarts a frame subscription a second and nothing says so, and one
/// that lets go of a decode without closing it leaves a reader on the relay for the life of the window.
/// </summary>
public sealed class BroadcastPreviewTests
{
    private const string Stream = "desk";

    /// <summary>
    /// A backend whose running state a test writes: what is publishing and whether the backend is previewing
    /// it.
    /// It records every receive effect and every frame subscription it is asked for, which is the whole of
    /// what the converge is judged on.
    ///
    /// It forwards everything else to <see cref="SeededBackend"/>, for the reason the other stand-ins here
    /// forward: a second set of answers would be a second fixture to keep in step with the first.
    /// </summary>
    private sealed class PreviewBackend : IBackend
    {
        private readonly SeededBackend _seed = new("windows");

        public event Action? Changed
        {
            add { }
            remove { }
        }

        /// <summary>What is publishing. Nothing by default, which the absent <c>Live</c> is what says.</summary>
        public PublishState Publish { get; set; } = new();

        /// <summary>
        /// Every relay decode a start or a stop was asked for, in the order it was asked.
        /// The local route must never add to either, and the end-to-end route is judged on exactly which
        /// pairs it opened and closed.
        /// </summary>
        public List<WatchKey> Started { get; } = [];

        public List<WatchKey> Stopped { get; } = [];

        /// <summary>
        /// Why the next start is refused, empty while none is.
        /// It is the leg that cannot carry this stream's format, which is the refusal the end-to-end route
        /// actually meets.
        /// </summary>
        public string StartRefusal { get; set; } = "";

        /// <summary>How many frame subscriptions were opened, per kind.</summary>
        public int PreviewSubscriptions { get; private set; }

        public int RelaySubscriptions { get; private set; }

        /// <summary>
        /// Opens one decode, and records it.
        /// The seed is what holds the open set, so what <see cref="ReceivingAsync"/> answers with is the
        /// effect this call had rather than a second list kept here.
        /// </summary>
        public Task StartReceiveAsync(
            string streamName, string transport, bool toneMap = false, CancellationToken cancellation = default)
        {
            Started.Add(new WatchKey { StreamName = streamName, Transport = transport });

            return StartRefusal.Length > 0
                ? Task.FromException(new BackendUnavailableException(StartRefusal))
                : _seed.StartReceiveAsync(streamName, transport, toneMap, cancellation);
        }

        public Task StopReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        {
            Stopped.Add(new WatchKey { StreamName = streamName, Transport = transport });
            return _seed.StopReceiveAsync(streamName, transport, cancellation);
        }

        // A fixture has no GPU and no pipeline, so what is recorded is the ask.
        // Refusing after that is the honest answer: a fake stream of handles would name GPU memory that does
        // not exist.
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

        // The wizard's screen pictures are a different screen's business.
        // This fixture is the broadcast card's, so the three calls are here to satisfy the seam and do
        // nothing.
        public Task StartMonitorPreviewAsync(int monitor, CancellationToken cancellation = default)
            => Task.CompletedTask;

        public Task StopMonitorPreviewAsync(int monitor, CancellationToken cancellation = default)
            => Task.CompletedTask;

        public Task<FrameChannel> OpenMonitorFramesAsync(int monitor, CancellationToken cancellation = default)
            => throw new BackendUnavailableException("this fixture lends no frames");

        public Task<IReadOnlyList<PreviewedMonitor>> PreviewedMonitorsAsync(CancellationToken cancellation = default)
            => Task.FromResult<IReadOnlyList<PreviewedMonitor>>([]);

        /// <summary>
        /// The preview carries no sound, so neither effect has anything to do here.
        /// They are answered rather than refused: a refusal is a state a test could mistake for the card's
        /// own, and what this fixture is about is which subscriptions were opened.
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

        public Task<IReadOnlyList<WatchKey>> WatchingAsync(CancellationToken cancellation = default)
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
        /// A stream that ends at once, so the session's first read lands and nothing arrives after it.
        /// What the running state is, is written on this fixture and re-read.
        /// </summary>
        public async IAsyncEnumerable<Event> SubscribeAsync(
            [EnumeratorCancellation] CancellationToken cancellation = default)
        {
            await Task.CompletedTask.ConfigureAwait(false);
            yield break;
        }
    }

    /// <summary>A stream in force under the name this suite uses, with a local preview behind it.</summary>
    private static PublishState Live(bool previewed = true)
    {
        var live = new PublishState.Types.Live { Publish = new PublishSettings { Name = Stream } };
        if (previewed)
        {
            live.Preview = new PublishState.Types.Preview { Port = 45678 };
        }

        return new PublishState { Live = live };
    }

    /// <summary>The preview as the backend reports it once a frame has left the pipeline.</summary>
    private static PublishState Decoding(bool live)
    {
        var state = Live();
        state.Live.Preview.Live = live;
        return state;
    }

    /// <summary>
    /// The leg the fixture's stored settings name for a viewer's tile, which is the one the end-to-end route
    /// receives over.
    /// </summary>
    private const string Leg = "srt";

    /// <summary>A card on screen, on the route it opens on.</summary>
    private static (PreviewViewModel Preview, Session Session) Showing(PreviewBackend backend)
        => Showing(backend, PreviewRoute.Local);

    /// <summary>
    /// A card on screen and on one route, over a session that has read the fixture once.
    /// The session is started rather than written to, because its fields are its own: every state a screen
    /// reads is one the backend answered with.
    /// </summary>
    private static (PreviewViewModel Preview, Session Session) Showing(PreviewBackend backend, PreviewRoute route)
    {
        var session = Read(backend);
        var preview = new PreviewViewModel(backend, Settings(backend, session), session, static action => action());

        preview.SetShowing(true);
        Choose(preview, route);
        Settle(preview, session);
        return (preview, session);
    }

    /// <summary>
    /// Lets the card see what its own effects did.
    /// What is decoding is the backend's answer and it moves when a decode is opened or closed; the app
    /// learns that off the event stream, and this fixture's event stream ends at once, so reading again is
    /// what stands in for it.
    /// </summary>
    private static void Settle(PreviewViewModel preview, Session session)
    {
        session.Start();
        preview.Apply();
    }

    /// <summary>
    /// Selects one route the way the toggle does: by handing the card the segment that stands for it, rather
    /// than by a setter of its own.
    /// </summary>
    private static void Choose(PreviewViewModel preview, PreviewRoute route)
        => preview.SelectedRoute = preview.Routes.Single(tab => tab.Value == route);

    /// <summary>
    /// A session that has read the fixture.
    /// Every answer is already completed and the dispatcher is straight through, so the read has landed by
    /// the time this returns.
    /// </summary>
    private static Session Read(PreviewBackend backend)
    {
        var session = new Session(backend, static action => action());
        session.Start();
        return session;
    }

    /// <summary>
    /// The settings the backend is holding, read the same way the window reads them.
    /// The card takes one value out of them - the leg its end-to-end route receives on - and the seeded
    /// answer names <see cref="Leg"/>.
    /// </summary>
    private static FormSession Settings(PreviewBackend backend, Session session)
        => new(backend, session, static action => action());

    [Fact]
    public void TheConvergeDrawsThePreviewTheBackendIsRunning()
    {
        var backend = new PreviewBackend { Publish = Live() };

        var (preview, _) = Showing(backend);

        Assert.NotNull(preview.Tile);
        Assert.True(preview.HasTile);
        Assert.Equal(Stream, preview.Tile.Name);
        Assert.True(preview.Tile.Source.IsPreview);
        Assert.Equal("", preview.Tile.Transport);
    }

    [Fact]
    public void ASecondPassOverUnchangedInputChangesNothing()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Showing(backend);

        var tile = preview.Tile;
        Assert.NotNull(tile);

        preview.Apply();
        preview.Apply();
        preview.SetShowing(true);

        // The same tile and not an equal one: a tile is a running frame subscription, so a rebuilt tile is a
        // restarted subscription however alike the two look.
        Assert.Same(tile, preview.Tile);
        Assert.Empty(backend.Started);
        Assert.Empty(backend.Stopped);
    }

    [Fact]
    public void TheCardDrawsNothingWhileItIsOffScreen()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var session = Read(backend);

        var preview = new PreviewViewModel(backend, Settings(backend, session), session, static action => action());

        Assert.Null(preview.Tile);
        Assert.False(preview.HasTile);
    }

    [Fact]
    public void LeavingTheScreenEndsTheSubscription()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Showing(backend);
        Assert.NotNull(preview.Tile);

        preview.SetShowing(false);

        Assert.Null(preview.Tile);
        Assert.False(preview.HasTile);
    }

    [Fact]
    public void GoingOffAirEndsTheSubscription()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, session) = Showing(backend);
        Assert.NotNull(preview.Tile);

        // The stream ended, and the session learns it the way it learns everything: by reading the backend
        // again.
        backend.Publish = new PublishState();
        session.Start();
        preview.Apply();

        Assert.Null(preview.Tile);
        Assert.False(preview.HasTile);
        Assert.Equal(Cards.PreviewNotPublishing, preview.Placeholder);
    }

    [Fact]
    public void ComingBackToSharingDrawsAgain()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Showing(backend);

        preview.SetShowing(false);
        preview.SetShowing(true);

        Assert.NotNull(preview.Tile);
        Assert.True(preview.Tile.Source.IsPreview);
    }

    [Fact]
    public void NothingPublishingIsItsOwnSentence()
    {
        var backend = new PreviewBackend();

        var (preview, _) = Showing(backend);

        Assert.Null(preview.Tile);
        Assert.True(preview.HasPlaceholder);
        Assert.Equal(Cards.PreviewNotPublishing, preview.Placeholder);
    }

    /// <summary>
    /// A stream on the air that the backend is not previewing is its own state, and it is a real one: a
    /// format with no local carriage, or a preview pipeline that would not start.
    /// The publish is untouched either way, which is the whole point of the leg being a copy.
    /// </summary>
    [Fact]
    public void APublishWithNoPreviewIsItsOwnSentence()
    {
        var backend = new PreviewBackend { Publish = Live(previewed: false) };

        var (preview, _) = Showing(backend);

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
            "Nothing is decoding this stream.",
            "Connecting.",
        };

        Assert.Equal(sentences.Length, sentences.Distinct().Count());
        Assert.All(sentences, sentence => Assert.NotEqual("", sentence));
    }

    /// <summary>
    /// The tile's own states are reached through the preview the backend reports, not through a decode list:
    /// the preview is part of the publish, so its pipeline's state travels with it.
    /// </summary>
    [Fact]
    public void ATileWithNoFrameYetSaysWhichStateItIsIn()
    {
        var backend = new PreviewBackend { Publish = Decoding(live: false) };
        var (preview, session) = Showing(backend);

        Assert.Equal("Connecting.", preview.Placeholder);

        backend.Publish = Decoding(live: true);
        session.Start();
        preview.Apply();

        Assert.Equal("", preview.Placeholder);
        Assert.False(preview.HasPlaceholder);
    }

    /// <summary>
    /// The sentence on the card carries the one thing a reader must not discover the hard way: this is what
    /// is being sent, and it says nothing about what viewers receive.
    /// </summary>
    [Fact]
    public void TheCardStatesWhatThePictureIsAndIsNot()
    {
        var backend = new PreviewBackend { Publish = Live() };

        var (preview, _) = Showing(backend);

        Assert.Equal(Cards.PreviewLocalCost, preview.Cost);
        Assert.Contains("never reaches the relay", preview.Cost);
        Assert.Contains("nothing about what viewers receive", preview.Cost);
    }

    // --- The route toggle -----------------------------------------------------------
    //
    // The card draws two pictures of one stream and the reader picks which.
    // What is asserted here is the half that is invisible on screen: which route asks the relay for a decode,
    // which one asks it for nothing, and that a switch between them leaves exactly one open.

    [Fact]
    public void TheToggleOffersEveryRouteAndOpensOnTheLocalOne()
    {
        var backend = new PreviewBackend { Publish = Live() };

        var (preview, _) = Showing(backend);

        Assert.Equal(PreviewRoutes.All, preview.Routes.Select(tab => tab.Value));
        Assert.Equal(PreviewRoute.Local, preview.SelectedRoute.Value);
        Assert.All(preview.Routes, tab => Assert.NotEqual("", tab.Label));
        Assert.NotEqual("", preview.RouteChoice);
    }

    /// <summary>
    /// The end-to-end route is a viewer of this machine's own stream, so it opens a decode exactly as the
    /// grid does: on the stream that is publishing, over the leg the stored settings name a tile receives on.
    /// </summary>
    [Fact]
    public void TheEndToEndRouteReceivesThisMachinesOwnStream()
    {
        var backend = new PreviewBackend { Publish = Live() };

        var (preview, _) = Showing(backend, PreviewRoute.EndToEnd);

        var opened = Assert.Single(backend.Started);
        Assert.Equal(Stream, opened.StreamName);
        Assert.Equal(Leg, opened.Transport);

        Assert.NotNull(preview.Tile);
        Assert.True(preview.Tile.Source.IsRelay);
        Assert.Equal(Leg, preview.Tile.Transport);
        Assert.True(preview.HasLeg);
    }

    /// <summary>
    /// Each route makes its own claim about the relay, and the two are opposite.
    /// A single sentence for both would be false under one of them.
    /// </summary>
    [Fact]
    public void EachRouteStatesItsOwnCost()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Showing(backend);

        Assert.Equal(Cards.PreviewLocalCost, preview.Cost);

        Choose(preview, PreviewRoute.EndToEnd);

        Assert.Equal(Cards.PreviewEndToEndCost, preview.Cost);
        Assert.NotEqual(Cards.PreviewLocalCost, Cards.PreviewEndToEndCost);
        Assert.Contains("what a viewer receives", preview.Cost);
    }

    /// <summary>
    /// The whole of what the toggle costs, asserted where it can be: switching to the local route closes the
    /// decode the other one opened, and switching back opens it again.
    /// A route that let go of the ask without closing would leave a reader slot on the relay for the life of
    /// the window.
    /// </summary>
    [Fact]
    public void SwitchingRoutesLeavesExactlyOneDecodeOpen()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, session) = Showing(backend, PreviewRoute.EndToEnd);

        Assert.Single(backend.Started);
        Assert.Empty(backend.Stopped);

        Choose(preview, PreviewRoute.Local);

        var closed = Assert.Single(backend.Stopped);
        Assert.Equal(Stream, closed.StreamName);
        Assert.Equal(Leg, closed.Transport);
        Assert.True(preview.Tile?.Source.IsPreview);

        Settle(preview, session);
        Choose(preview, PreviewRoute.EndToEnd);

        Assert.Equal(2, backend.Started.Count);
        Assert.Single(backend.Stopped);
    }

    /// <summary>
    /// The local route asks the relay for nothing, which is the property the loopback leg exists for: no
    /// decode is opened, none is closed, and no relay frame subscription is made, so the relay serves no
    /// reader for that picture and counts none.
    /// </summary>
    [Fact]
    public async Task TheLocalRouteAsksTheRelayForNothing()
    {
        var backend = new PreviewBackend { Publish = Live() };

        var (preview, _) = Showing(backend);
        Assert.NotNull(preview.Tile);

        await Assert.ThrowsAsync<BackendUnavailableException>(
            () => preview.Tile.OpenAsync(CancellationToken.None));

        Assert.Equal(1, backend.PreviewSubscriptions);
        Assert.Equal(0, backend.RelaySubscriptions);
        Assert.Empty(backend.Started);
        Assert.Empty(backend.Stopped);
    }

    /// <summary>
    /// A render pass on the end-to-end route asks for nothing it has already asked for.
    /// The pass runs on every event the session reports, so a converge that re-asked would spend a round trip
    /// a second on a decode that is already open.
    /// </summary>
    [Fact]
    public void ASecondPassOnTheEndToEndRouteAsksForNothing()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Showing(backend, PreviewRoute.EndToEnd);

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
    /// Leaving the screen closes the decode, and not only the subscription.
    /// The reader slot is what the end-to-end route costs the relay, so a card nobody is looking at that went
    /// on holding one would be spending a viewer's bandwidth on a picture nothing draws.
    /// </summary>
    [Fact]
    public void LeavingTheScreenClosesTheEndToEndDecode()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Showing(backend, PreviewRoute.EndToEnd);

        preview.SetShowing(false);

        Assert.Single(backend.Stopped);
        Assert.Null(preview.Tile);
    }

    /// <summary>
    /// A decode the viewer's grid is also drawing is left running.
    /// One pipeline serves both windows, so a stop from this card would take the picture out of a tile the
    /// reader is looking at on the other screen.
    /// </summary>
    [Fact]
    public void ADecodeTheGridIsDrawingIsLeftOpen()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var (preview, _) = Showing(backend, PreviewRoute.EndToEnd);

        preview.SetGridLeg(stream => stream == Stream ? Leg : "");
        preview.SetShowing(false);

        Assert.Single(backend.Started);
        Assert.Empty(backend.Stopped);
    }

    /// <summary>
    /// A refused start is shown as the backend wrote it.
    /// A leg that cannot carry this stream's format names the format and the protocols that would have
    /// carried it, which is the whole of what makes the refusal actionable - and is nothing this side could
    /// compose.
    /// </summary>
    [Fact]
    public void ARefusedDecodeSaysWhyInTheBackendsWords()
    {
        const string refusal = "srt does not carry av1: rtsp and webrtc do.";
        var backend = new PreviewBackend { Publish = Live(), StartRefusal = refusal };

        var (preview, _) = Showing(backend, PreviewRoute.EndToEnd);

        Assert.Equal(refusal, preview.Placeholder);
        Assert.True(preview.HasPlaceholder);
        Assert.Null(preview.Tile);

        // Asked once and not again.
        // A refusal is a fact about the key rather than a moment, so re-asking every pass would be a round
        // trip a second against a leg that has answered.
        preview.Apply();
        preview.Apply();
        Assert.Single(backend.Started);
    }

    /// <summary>
    /// Switching off a refused route clears its sentence.
    /// Held across the switch, it would stand under the other route's picture describing a leg that route
    /// never uses.
    /// </summary>
    [Fact]
    public void LeavingARefusedRouteClearsItsSentence()
    {
        var backend = new PreviewBackend
        {
            Publish = Decoding(live: true),
            StartRefusal = "srt does not carry av1.",
        };
        var (preview, _) = Showing(backend, PreviewRoute.EndToEnd);
        Assert.Equal(backend.StartRefusal, preview.Placeholder);

        Choose(preview, PreviewRoute.Local);

        Assert.Equal("", preview.Placeholder);
        Assert.False(preview.HasPlaceholder);
        Assert.True(preview.Tile?.Source.IsPreview);
    }
}
