using System.Runtime.CompilerServices;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Broadcast.Preview.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The broadcast preview draws what is being sent, from a copy that never leaves this
/// machine.
///
/// <b>What these state is that the relay is not a party to it.</b> The preview used to be a
/// loopback - a decode of this machine's own stream, opened with <c>StartReceive</c> and read
/// back off the relay - which worked and cost the broadcast screen its own figures: the card
/// occupied a reader slot, so a stream nobody was watching reported one viewer and the
/// worst-viewer plot described the publisher's own round trip. The publish child now copies
/// its encoded video to a loopback port and the backend decodes that, so there is no receive
/// effect for this card to call and no reader for the relay to count. That is asserted here
/// against the calls the seam received rather than against what the card drew, because a
/// receive effect creeping back in is invisible on screen.
///
/// <b>The rest is the lifecycle</b>, which is the other part that can go wrong invisibly: a
/// converge that rebuilds the tile on every render pass restarts a frame subscription a second
/// and nothing says so.
/// </summary>
public sealed class BroadcastPreviewTests
{
    private const string Stream = "desk";

    /// <summary>
    /// A backend whose running state a test writes: what is publishing and whether the backend
    /// is previewing it. It records every receive effect and every frame subscription it is
    /// asked for, which is the whole of what the converge is judged on.
    ///
    /// It forwards everything else to <see cref="SeededBackend"/>, for the reason the other
    /// stand-ins here forward: a second set of answers would be a second fixture to keep in
    /// step with the first.
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

        /// <summary>Every relay decode a start or a stop was asked for. The preview must never add to either.</summary>
        public List<WatchKey> Started { get; } = [];

        public List<WatchKey> Stopped { get; } = [];

        /// <summary>How many frame subscriptions were opened, per kind.</summary>
        public int PreviewSubscriptions { get; private set; }

        public int RelaySubscriptions { get; private set; }

        public Task StartReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        {
            Started.Add(new WatchKey { StreamName = streamName, Transport = transport });
            return Task.CompletedTask;
        }

        public Task StopReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        {
            Stopped.Add(new WatchKey { StreamName = streamName, Transport = transport });
            return Task.CompletedTask;
        }

        // A fixture has no GPU and no pipeline, so what is recorded is the ask. Refusing after
        // that is the honest answer: a fake stream of handles would name GPU memory that does
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

        /// <summary>
        /// The preview carries no sound, so neither effect has anything to do here. They are
        /// answered rather than refused: a refusal is a state a test could mistake for the
        /// card's own, and what this fixture is about is which subscriptions were opened.
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

        public Task ApplyToStreamAsync(Settings settings, CancellationToken cancellation = default)
            => _seed.ApplyToStreamAsync(settings, cancellation);

        public Task StopPublishAsync(CancellationToken cancellation = default)
            => _seed.StopPublishAsync(cancellation);

        public Task<double> MeasureUplinkAsync(CancellationToken cancellation = default)
            => _seed.MeasureUplinkAsync(cancellation);

        public Task StartWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
            => _seed.StartWatchAsync(streamName, transport, cancellation);

        public Task StopWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
            => _seed.StopWatchAsync(streamName, transport, cancellation);

        public Task OpenLogAsync(string path, CancellationToken cancellation = default)
            => _seed.OpenLogAsync(path, cancellation);

        public Task OpenLogsFolderAsync(CancellationToken cancellation = default)
            => _seed.OpenLogsFolderAsync(cancellation);

        /// <summary>
        /// A stream that ends at once, so the session's first read lands and nothing arrives
        /// after it. What the running state is, is written on this fixture and re-read.
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
    /// A card on screen, over a session that has read the fixture once. The session is started
    /// rather than written to, because its fields are its own: every state a screen reads is
    /// one the backend answered with.
    /// </summary>
    private static (PreviewViewModel Preview, Session Session) Showing(PreviewBackend backend)
    {
        var session = Read(backend);
        var preview = new PreviewViewModel(backend, session, static action => action());

        preview.SetShowing(true);
        return (preview, session);
    }

    /// <summary>
    /// A session that has read the fixture. Every answer is already completed and the
    /// dispatcher is straight through, so the read has landed by the time this returns.
    /// </summary>
    private static Session Read(PreviewBackend backend)
    {
        var session = new Session(backend, static action => action());
        session.Start();
        return session;
    }

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

    /// <summary>
    /// The point of the change, asserted where it can be asserted: the relay is asked for
    /// nothing. No decode is opened, none is closed, and no relay frame subscription is made,
    /// so the relay serves no reader for this picture and counts none.
    /// </summary>
    [Fact]
    public async Task ThePreviewAsksTheRelayForNothing()
    {
        var backend = new PreviewBackend { Publish = Live() };

        var (preview, _) = Showing(backend);
        Assert.NotNull(preview.Tile);

        // The subscription is the control's, so it is opened here the way the control opens
        // it. What is being asserted is which of the two calls it lands on.
        await Assert.ThrowsAsync<BackendUnavailableException>(
            () => preview.Tile.OpenAsync(CancellationToken.None));

        Assert.Equal(1, backend.PreviewSubscriptions);
        Assert.Equal(0, backend.RelaySubscriptions);
        Assert.Empty(backend.Started);
        Assert.Empty(backend.Stopped);
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

        // The same tile and not an equal one: a tile is a running frame subscription, so a
        // rebuilt tile is a restarted subscription however alike the two look.
        Assert.Same(tile, preview.Tile);
        Assert.Empty(backend.Started);
        Assert.Empty(backend.Stopped);
    }

    [Fact]
    public void TheCardDrawsNothingWhileItIsOffScreen()
    {
        var backend = new PreviewBackend { Publish = Live() };
        var session = Read(backend);

        var preview = new PreviewViewModel(backend, session, static action => action());

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

        // The stream ended, and the session learns it the way it learns everything: by
        // reading the backend again.
        backend.Publish = new PublishState();
        session.Start();
        preview.Apply();

        Assert.Null(preview.Tile);
        Assert.False(preview.HasTile);
        Assert.Equal(Cards.PreviewNotPublishing, preview.Placeholder);
    }

    [Fact]
    public void ComingBackOnAirDrawsAgain()
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
    /// A stream on the air that the backend is not previewing is its own state, and it is a
    /// real one: a format with no local carriage, or a preview pipeline that would not start.
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
            "Nothing is decoding this stream.",
            "Connecting.",
        };

        Assert.Equal(sentences.Length, sentences.Distinct().Count());
        Assert.All(sentences, sentence => Assert.NotEqual("", sentence));
    }

    /// <summary>
    /// The tile's own states are reached through the preview the backend reports, not through
    /// a decode list: the preview is part of the publish, so its pipeline's state travels with
    /// it.
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
    /// The sentence on the card carries the one thing a reader must not discover the hard way:
    /// this is what is being sent, and it says nothing about what viewers receive.
    /// </summary>
    [Fact]
    public void TheCardStatesWhatThePictureIsAndIsNot()
    {
        var backend = new PreviewBackend { Publish = Live() };

        var (preview, _) = Showing(backend);

        Assert.Equal(Cards.PreviewCost, preview.Cost);
        Assert.Contains("never reaches the relay", preview.Cost);
        Assert.Contains("nothing about what viewers receive", preview.Cost);
    }
}
