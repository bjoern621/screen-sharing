using Grpc.Core;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Backend;

/// <summary>
/// One consumer's subscription to one decode's frames: the second service on the same
/// socket, carrying handles and never pixels (<c>docs/ipc-api.md</c>).
///
/// <b>It draws nothing and imports nothing.</b> What it owns is the call: the subscribe that
/// opens it, the releases and render sizes that go back, and the events that come out. Which
/// handle type this machine's compositor can open, and what happens to a slot once it is
/// imported, belong to the control that draws (<c>Features/Viewer/Tile</c>).
///
/// <b>The protocol is a loan, and the release is the whole of the flow control.</b> Every
/// frame handed over takes a slot out of the backend's pool and the slot comes back only when
/// this side says so. A consumer that stops releasing slows to its own rate and never sees a
/// half-written picture; it does not stall the decode, which goes on running and drops the
/// frames it has nowhere to put. That is why the release travels on this call rather than on a
/// second one: a release that outlived its subscription would free a slot of a pool that is
/// gone.
///
/// <b>It opens no decode.</b> <see cref="IBackend.StartReceiveAsync"/> is the effect that opens
/// a relay decode and the publish itself is what opens its local preview, and a subscription to
/// a decode nothing is running is refused. The two staying separate is what lets a decode
/// outlive the window drawing it.
///
/// <b>Which of the two a call draws from is its first message and nothing else.</b> A relay
/// decode is named by a stream and a leg; the running publish's preview is named by nothing,
/// because there is at most one publish and the preview is part of it. Everything after that
/// first message is one protocol.
/// </summary>
public sealed class FrameChannel : IAsyncDisposable
{
    private readonly AsyncDuplexStreamingCall<FramesRequest, FrameEvent> _call;

    /// <summary>
    /// Serializes the writes. Releases come from wherever a frame finished drawing and render
    /// sizes from wherever the tile was measured, and a gRPC request stream tolerates one
    /// writer at a time.
    /// </summary>
    private readonly SemaphoreSlim _writing = new(1, 1);

    private bool _closed;

    private FrameChannel(AsyncDuplexStreamingCall<FramesRequest, FrameEvent> call)
    {
        _call = call;
    }

    /// <summary>
    /// Subscribes to the frames of one running decode.
    ///
    /// The subscribe is written before this returns, so a caller that starts reading has
    /// already asked for something. What comes back first is the pool, and no frame precedes
    /// it: a consumer cannot be handed a slot it has not been told how to open.
    /// </summary>
    public static Task<FrameChannel> OpenAsync(
        FrameService.FrameServiceClient client,
        string streamName,
        string transport,
        CancellationToken cancellation)
    {
        Assert.That(streamName.Length > 0, "a frame channel names the stream it draws");
        Assert.That(transport.Length > 0, "a frame channel names the leg the stream is decoded from", streamName);

        return SubscribeAsync(client, new FrameSubscribe
        {
            Stream = new WatchKey { StreamName = streamName, Transport = transport },
        }, cancellation);
    }

    /// <summary>
    /// Subscribes to the frames of the running publish's local preview.
    ///
    /// It names no stream and no leg, and neither is an omission. The backend runs at most one
    /// publish, so "the preview" is already a complete identity; and what it draws never
    /// crossed the relay, so there is no protocol to name it by
    /// (<c>docs/viewer-architecture.md</c>, "What the broadcast preview draws").
    ///
    /// It opens no pipeline, exactly as <see cref="OpenAsync"/> opens no decode. What brings
    /// the preview up is the publish itself, so a call made while nothing is publishing is
    /// refused rather than served - and a caller reads the publish state to know whether to
    /// ask at all.
    /// </summary>
    public static Task<FrameChannel> OpenPreviewAsync(
        FrameService.FrameServiceClient client,
        CancellationToken cancellation)
        => SubscribeAsync(client, new FrameSubscribe { PublishPreview = new PublishPreview() }, cancellation);

    /// <summary>
    /// Opens the call and says what it is for. One method for both kinds of subscription,
    /// because everything after the first message is the same protocol: the two differ in
    /// which arm of the oneof they fill and in nothing else.
    /// </summary>
    private static async Task<FrameChannel> SubscribeAsync(
        FrameService.FrameServiceClient client,
        FrameSubscribe subscribe,
        CancellationToken cancellation)
    {
        Assert.NotNull(client, "a frame channel is opened on the frame service");
        Assert.NotNull(subscribe, "a frame channel says which decode it is for");

        var call = client.Frames(cancellationToken: cancellation);
        var channel = new FrameChannel(call);

        try
        {
            await call.RequestStream.WriteAsync(new FramesRequest { Subscribe = subscribe }, cancellation)
                .ConfigureAwait(false);
        }
        catch
        {
            // The call is this method's until it is handed over. A subscribe that failed
            // leaves a call nobody holds, which would keep the backend's side alive until the
            // connection itself went.
            await channel.DisposeAsync().ConfigureAwait(false);
            throw;
        }

        return channel;
    }

    /// <summary>
    /// What the backend says, in order, until the subscription ends. The enumeration
    /// completes when the decode ends or the call is disposed.
    /// </summary>
    public IAsyncEnumerable<FrameEvent> ReadAsync(CancellationToken cancellation)
        => _call.ResponseStream.ReadAllAsync(cancellation);

    /// <summary>
    /// Hands one slot back, naming the pool it came from.
    ///
    /// The generation is echoed rather than assumed: a pool is re-announced whenever the
    /// pipeline renegotiates, and a release that crossed that announcement on the wire names a
    /// pool that no longer exists. The backend discards it rather than freeing a slot of the
    /// pool that replaced it.
    /// </summary>
    public Task ReleaseAsync(ulong generation, uint slot, ulong serial)
        => WriteAsync(new FramesRequest
        {
            Release = new FrameRelease { Generation = generation, Slot = slot, Serial = serial },
        });

    /// <summary>
    /// Says how many pixels this consumer will draw the frames at.
    ///
    /// It is a bound and not a size: the receive pipeline's scaler fixates inside it and
    /// corrects the pixel aspect ratio, so a tile far smaller than its stream has the
    /// conversion done at its own size instead of the source's, and a tile larger than the
    /// stream gets the stream's own size rather than an upscale nobody asked for.
    /// </summary>
    public Task RenderSizeAsync(int width, int height)
        => WriteAsync(new FramesRequest
        {
            RenderSize = new FrameRenderSize { Width = width, Height = height },
        });

    /// <summary>
    /// Ends the subscription. The backend frees the pool as the call ends, so every handle
    /// this side imported names nothing afterwards - which is exactly why the imports are
    /// disposed by whoever made them before this runs.
    /// </summary>
    public async ValueTask DisposeAsync()
    {
        if (_closed)
        {
            return;
        }

        _closed = true;
        try
        {
            await _call.RequestStream.CompleteAsync().ConfigureAwait(false);
        }
        catch (Exception)
        {
            // A call that is already broken is a call that is already ended, which is what
            // this method was asked for. The dispose below is what actually releases it.
        }

        _call.Dispose();
        _writing.Dispose();
    }

    /// <summary>
    /// One write, serialized against the others.
    ///
    /// A failed write is swallowed. Every one of them is a message about a stream that has
    /// ended - a release of a slot nobody holds, a size for a pipeline that is gone - and the
    /// reader is where the end is learned and reported, so raising here would report it twice
    /// and from the side that knows less.
    /// </summary>
    private async Task WriteAsync(FramesRequest request)
    {
        if (_closed)
        {
            return;
        }

        await _writing.WaitAsync().ConfigureAwait(false);
        try
        {
            if (!_closed)
            {
                await _call.RequestStream.WriteAsync(request).ConfigureAwait(false);
            }
        }
        catch (Exception)
        {
        }
        finally
        {
            _writing.Release();
        }
    }
}
