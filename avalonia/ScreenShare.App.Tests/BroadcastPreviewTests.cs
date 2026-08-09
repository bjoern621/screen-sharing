using System.Runtime.CompilerServices;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Broadcast.Preview.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The broadcast preview receives this machine's own stream back off the relay, on the one
/// frame path there is.
///
/// What these state is the lifecycle, because the lifecycle is the part that can go wrong
/// invisibly: a converge that opens a second decode on every render pass costs a round trip a
/// second and nothing on screen says so, and a converge that closes a shared decode takes a
/// tile off the viewer screen with it. Both are asserted against the calls the seam received
/// rather than against what the card drew.
/// </summary>
public sealed class BroadcastPreviewTests
{
    private const string Stream = "desk";
    private const string Leg = "srt";

    /// <summary>
    /// A backend whose running state a test writes: what is publishing, what the relay is
    /// carrying, what is decoding, and which leg the settings resolve to. It records every
    /// receive effect it is asked for, which is the whole of what the converge is judged on.
    ///
    /// It forwards everything else to <see cref="SeededBackend"/>, for the reason the other
    /// stand-ins here forward: a second set of answers would be a second fixture to keep in
    /// step with the first.
    /// </summary>
    private sealed class ReceivingBackend : IBackend
    {
        private readonly SeededBackend _seed = new("windows");

        public event Action? Changed
        {
            add { }
            remove { }
        }

        /// <summary>What the settings resolve the tile watch leg to. Empty for a form that carries no such field.</summary>
        public string TileLeg { get; set; } = Leg;

        /// <summary>What is publishing. Nothing by default, which the absent <c>Live</c> is what says.</summary>
        public PublishState Publish { get; set; } = new();

        /// <summary>What the relay is carrying. Reachable and empty by default.</summary>
        public RelayStatus Relay { get; set; } = new() { Reachable = true };

        /// <summary>What the backend is decoding, whole like every other state it sends.</summary>
        public IReadOnlyList<ReceiveStream> Receiving { get; set; } = [];

        /// <summary>Why a start is refused, empty while one is accepted.</summary>
        public string Refusal { get; set; } = "";

        /// <summary>Every decode a start was asked for, in order.</summary>
        public List<WatchKey> Started { get; } = [];

        /// <summary>Every decode a stop was asked for, in order. The preview must never add to it.</summary>
        public List<WatchKey> Stopped { get; } = [];

        /// <summary>A start asked for and not answered, so the card can be read mid-round-trip.</summary>
        private TaskCompletionSource? _held;

        public void HoldStarts() => _held = new TaskCompletionSource();

        public void AnswerStarts()
        {
            var held = _held ?? throw new InvalidOperationException("no start is being held");

            _held = null;
            Answers.Now(held.SetResult);
        }

        public Task StartReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        {
            Started.Add(new WatchKey { StreamName = streamName, Transport = transport });

            return Refusal.Length > 0
                ? Task.FromException(new BackendUnavailableException(Refusal))
                : _held?.Task ?? Task.CompletedTask;
        }

        public Task StopReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        {
            Stopped.Add(new WatchKey { StreamName = streamName, Transport = transport });
            return Task.CompletedTask;
        }

        public Task<PublishState> PublishStateAsync(CancellationToken cancellation = default)
            => Task.FromResult(Publish);

        public Task<RelayStatus> RelayStatusAsync(CancellationToken cancellation = default)
            => Task.FromResult(Relay);

        public Task<IReadOnlyList<ReceiveStream>> ReceivingAsync(CancellationToken cancellation = default)
            => Task.FromResult(Receiving);

        /// <summary>
        /// A form carrying the one field the tile leg is read out of. The seed's own form has
        /// no viewer group at all, and the leg is exactly what this suite is about.
        /// </summary>
        public Task<Form> ResolveFormAsync(Settings draft, CancellationToken cancellation = default)
        {
            var form = new Form { Settings = draft.Clone() };
            var group = new FieldGroup { Key = "viewer" };

            if (TileLeg.Length > 0)
            {
                group.Fields.Add(new Field
                {
                    Key = "viewer.tile_watch_transport",
                    Control = ControlKind.Select,
                    Visible = true,
                    Enabled = true,
                    Value = new FieldValue { Text = TileLeg },
                });
            }

            form.Groups.Add(group);
            return Task.FromResult(form);
        }

        public Task<Catalog> CatalogAsync(CancellationToken cancellation = default)
            => _seed.CatalogAsync(cancellation);

        public Task<Settings> SettingsAsync(CancellationToken cancellation = default)
            => _seed.SettingsAsync(cancellation);

        public Task<IReadOnlyList<WatchKey>> WatchingAsync(CancellationToken cancellation = default)
            => _seed.WatchingAsync(cancellation);

        public Task StartPublishAsync(Settings settings, CancellationToken cancellation = default)
            => _seed.StartPublishAsync(settings, cancellation);

        public Task StopPublishAsync(CancellationToken cancellation = default)
            => _seed.StopPublishAsync(cancellation);

        public Task<double> MeasureUplinkAsync(CancellationToken cancellation = default)
            => _seed.MeasureUplinkAsync(cancellation);

        public Task StartWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
            => _seed.StartWatchAsync(streamName, transport, cancellation);

        public Task StopWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
            => _seed.StopWatchAsync(streamName, transport, cancellation);

        public Task<FrameChannel> OpenFramesAsync(string streamName, string transport, CancellationToken cancellation = default)
            => _seed.OpenFramesAsync(streamName, transport, cancellation);

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

    /// <summary>A stream in force under the name this suite uses.</summary>
    private static PublishState Live() => new()
    {
        Live = new PublishState.Types.Live { Publish = new PublishSettings { Name = Stream } },
    };

    /// <summary>A relay carrying one path by that name, which is what makes a decode worth asking for.</summary>
    private static RelayStatus Carrying()
    {
        var relay = new RelayStatus { Reachable = true };
        relay.Paths.Add(new RelayPath { Name = Stream, Ready = true });
        return relay;
    }

    private static ReceiveStream Decoding(bool live) => new()
    {
        Stream = new WatchKey { StreamName = Stream, Transport = Leg },
        Live = live,
    };

    /// <summary>
    /// A card on screen, over a session that has read the fixture once. The session is started
    /// rather than written to, because its fields are its own: every state a screen reads is
    /// one the backend answered with.
    /// </summary>
    private static (PreviewViewModel Preview, Session Session) Showing(ReceivingBackend backend)
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
    private static Session Read(ReceivingBackend backend)
    {
        var session = new Session(backend, static action => action());
        session.Start();
        return session;
    }

    [Fact]
    public void TheConvergeOpensOneDecodeForTheStreamThatIsPublishing()
    {
        var backend = new ReceivingBackend { Publish = Live(), Relay = Carrying() };

        var (preview, _) = Showing(backend);

        Assert.Single(backend.Started);
        Assert.Equal(Stream, backend.Started[0].StreamName);
        Assert.Equal(Leg, backend.Started[0].Transport);
        Assert.NotNull(preview.Tile);
        Assert.True(preview.HasTile);
    }

    [Fact]
    public void ASecondPassOverUnchangedInputAsksForNothing()
    {
        var backend = new ReceivingBackend { Publish = Live(), Relay = Carrying() };
        var (preview, _) = Showing(backend);

        preview.Apply();
        preview.Apply();
        preview.SetShowing(true);

        Assert.Single(backend.Started);
    }

    [Fact]
    public void APassWhileTheStartIsStillOutAsksForNothingMore()
    {
        var backend = new ReceivingBackend { Publish = Live(), Relay = Carrying() };
        backend.HoldStarts();

        var (preview, _) = Showing(backend);

        preview.Apply();
        Assert.Single(backend.Started);
        Assert.Null(preview.Tile);
        Assert.Equal(Cards.PreviewOpening, preview.Placeholder);

        backend.AnswerStarts();

        Assert.Single(backend.Started);
        Assert.NotNull(preview.Tile);
    }

    [Fact]
    public void TheDecodeIsNotOpenedWhileTheCardIsOffScreen()
    {
        var backend = new ReceivingBackend { Publish = Live(), Relay = Carrying() };
        var session = Read(backend);

        var preview = new PreviewViewModel(backend, session, static action => action());

        Assert.Empty(backend.Started);
        Assert.Null(preview.Tile);
    }

    [Fact]
    public void LeavingTheScreenDropsTheSubscriptionAndLeavesTheDecodeAlone()
    {
        var backend = new ReceivingBackend { Publish = Live(), Relay = Carrying() };
        var (preview, _) = Showing(backend);
        Assert.NotNull(preview.Tile);

        preview.SetShowing(false);

        Assert.Null(preview.Tile);
        Assert.False(preview.HasTile);
        Assert.Empty(backend.Stopped);
    }

    [Fact]
    public void GoingOffAirDropsTheSubscriptionAndLeavesTheDecodeAlone()
    {
        var backend = new ReceivingBackend { Publish = Live(), Relay = Carrying() };
        var (preview, session) = Showing(backend);
        Assert.NotNull(preview.Tile);

        // The stream ended, and the session learns it the way it learns everything: by
        // reading the backend again.
        backend.Publish = new PublishState();
        session.Start();
        preview.Apply();

        Assert.Null(preview.Tile);
        Assert.Empty(backend.Stopped);
        Assert.Equal(Cards.PreviewNotPublishing, preview.Placeholder);
    }

    [Fact]
    public void ComingBackOnAirAsksAgain()
    {
        var backend = new ReceivingBackend { Publish = Live(), Relay = Carrying() };
        var (preview, _) = Showing(backend);

        preview.SetShowing(false);
        preview.SetShowing(true);

        Assert.Equal(2, backend.Started.Count);
        Assert.NotNull(preview.Tile);
    }

    [Fact]
    public void NothingPublishingIsItsOwnSentence()
    {
        var backend = new ReceivingBackend { Relay = Carrying() };

        var (preview, _) = Showing(backend);

        Assert.Empty(backend.Started);
        Assert.True(preview.HasPlaceholder);
        Assert.Equal(Cards.PreviewNotPublishing, preview.Placeholder);
    }

    [Fact]
    public void AnUnresolvedLegIsItsOwnSentence()
    {
        var backend = new ReceivingBackend { TileLeg = "", Publish = Live(), Relay = Carrying() };

        var (preview, _) = Showing(backend);

        Assert.Empty(backend.Started);
        Assert.Equal(Cards.PreviewNoLeg, preview.Placeholder);
    }

    [Fact]
    public void ARelayThatHasNotPickedThePathUpIsItsOwnSentence()
    {
        var backend = new ReceivingBackend { Publish = Live(), Relay = new RelayStatus { Reachable = true } };

        var (preview, _) = Showing(backend);

        Assert.Empty(backend.Started);
        Assert.Equal(Cards.PreviewRelayHasNoPath, preview.Placeholder);
    }

    [Fact]
    public void ThePlaceholderStatesAreDistinct()
    {
        var sentences = new[]
        {
            Cards.PreviewNotPublishing,
            Cards.PreviewNoLeg,
            Cards.PreviewRelayHasNoPath,
            Cards.PreviewOpening,
        };

        Assert.Equal(sentences.Length, sentences.Distinct().Count());
        Assert.All(sentences, sentence => Assert.NotEqual("", sentence));
    }

    [Fact]
    public void ARefusedStartShowsTheBackendsOwnSentence()
    {
        const string refusal = "cannot receive 'desk' over srt: desk is vp9, which srt cannot carry: watch it over rtsp";
        var backend = new ReceivingBackend { Publish = Live(), Relay = Carrying(), Refusal = refusal };

        var (preview, _) = Showing(backend);

        Assert.Null(preview.Tile);
        Assert.Equal(refusal, preview.Placeholder);
    }

    [Fact]
    public void ATileWithNoFrameYetSaysWhichStateItIsIn()
    {
        var backend = new ReceivingBackend { Publish = Live(), Relay = Carrying() };
        var (preview, session) = Showing(backend);

        // The decode has been opened and the backend has not reported it yet, which is not
        // the same state as a pipeline that is up with no frame out of it.
        Assert.Equal("Nothing is decoding this stream.", preview.Placeholder);

        backend.Receiving = [Decoding(live: false)];
        session.Start();
        preview.Apply();

        Assert.Equal("Connecting.", preview.Placeholder);

        backend.Receiving = [Decoding(live: true)];
        session.Start();
        preview.Apply();

        Assert.Equal("", preview.Placeholder);
        Assert.False(preview.HasPlaceholder);
    }

    [Fact]
    public void TheCardStatesWhatTheLoopbackCosts()
    {
        var backend = new ReceivingBackend { Publish = Live(), Relay = Carrying() };

        var (preview, _) = Showing(backend);

        Assert.Equal(Cards.PreviewCost(Words.Transport(Leg)), preview.Cost);
        Assert.Contains("SRT", preview.Cost);
        Assert.Contains("bandwidth", preview.Cost);
    }
}
