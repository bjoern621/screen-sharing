using Grpc.Core;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Backend;

/// <summary>
/// One consumer's subscription to one decode's frames, over the second service on the control socket: handles,
/// never pixels (<c>docs/ipc-api.md</c>).
///
/// <b>Draws nothing and imports nothing.</b> What it owns is the call: the subscribe that opens it, the releases
/// and render sizes going back, and the events coming out.
/// Which handle type this machine's compositor can open, and what becomes of an imported slot, belong to the
/// control that draws (<c>Features/Viewer/Tile</c>).
///
/// <b>The protocol is a loan, and the release is the whole of the flow control.</b> Every frame handed over takes
/// a slot out of the backend's pool, and the slot comes back only when this side says so.
/// A consumer that stops releasing slows to its own rate and never sees a half-written picture, and the decode
/// runs on and drops the frames it has nowhere to put.
/// The release rides this call rather than a second one, one outliving its subscription freeing a slot of a pool
/// that is gone.
///
/// <b>Opens no picture.</b> <see cref="IBackend.StartReceiveAsync"/> opens a relay decode, the publish opens its
/// own preview, <see cref="IBackend.StartMonitorPreviewAsync"/> opens a monitor's, and a subscription to a
/// picture nothing is producing is refused.
/// The separation lets a decode outlive the window drawing it.
///
/// <b>The first message names which picture, and nothing after it differs.</b> A relay decode goes by stream and
/// leg, the running publish's preview by nothing at all since there is at most one publish, a monitor by its
/// index.
/// </summary>
public sealed class FrameChannel : IAsyncDisposable
{
    private readonly AsyncDuplexStreamingCall<FramesRequest, FrameEvent> _call;

    /// <summary>
    /// One writer at a time, which is all a gRPC request stream takes.
    /// A release comes from wherever a frame finished drawing and a render size from wherever the tile was
    /// measured.
    /// </summary>
    private readonly SemaphoreSlim _writing = new(1, 1);

    private bool _closed;

    private FrameChannel(AsyncDuplexStreamingCall<FramesRequest, FrameEvent> call)
    {
        _call = call;
    }

    /// <summary>
    /// Subscribes to one running relay decode's frames.
    /// The subscribe is written before this returns, so a caller that starts reading has already asked.
    /// The pool comes back first and no frame precedes it: a slot cannot be handed over before the way to open it
    /// is.
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
            Stream = new StreamRef { StreamName = streamName, Transport = transport },
        }, cancellation);
    }

    /// <summary>
    /// Subscribes to the running publish's local preview.
    /// No stream and no leg, and neither is an omission: the backend runs at most one publish, so "the preview"
    /// is a complete identity, and what it draws never crossed the relay, so no protocol names it
    /// (<c>docs/viewer-architecture.md</c>, "What the broadcast preview draws").
    /// Opens no pipeline, as <see cref="OpenAsync"/> opens no decode.
    /// The publish brings the preview up, so a call made while nothing is publishing is refused, and a caller
    /// reads the publish state to know whether to ask at all.
    /// </summary>
    public static Task<FrameChannel> OpenPreviewAsync(
        FrameService.FrameServiceClient client,
        CancellationToken cancellation)
        => SubscribeAsync(client, new FrameSubscribe { PublishPreview = new PublishPreview() }, cancellation);

    /// <summary>
    /// Subscribes to one of this machine's monitors, read live so a screen can be picked by looking at it.
    /// The index is the whole identity: <c>publish.monitor</c> holds it and the catalog enumerates outputs under
    /// it, so a size or a name here would send the catalog back.
    /// Opens no capture, as <see cref="OpenAsync"/> opens no decode.
    /// <see cref="IBackend.StartMonitorPreviewAsync"/> reads the screen, so a call for a monitor nothing is
    /// previewing is refused.
    /// </summary>
    public static Task<FrameChannel> OpenMonitorAsync(
        FrameService.FrameServiceClient client,
        int monitor,
        CancellationToken cancellation)
        => SubscribeAsync(client, new FrameSubscribe
        {
            MonitorPreview = new MonitorPreview { Monitor = monitor },
        }, cancellation);

    /// <summary>
    /// Opens the call and says what it is for.
    /// One method whatever the subscription names, the kinds differing in which arm of the oneof they fill and in
    /// nothing else.
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
            // The call belongs to this method until it is handed over.
            // A failed subscribe would leave a call nobody holds, keeping the backend's side alive until the
            // connection itself went.
            await channel.DisposeAsync().ConfigureAwait(false);
            throw;
        }

        return channel;
    }

    /// <summary>
    /// What the backend says, in order, until the subscription ends.
    /// Completes when the decode ends or the call is disposed.
    /// </summary>
    public IAsyncEnumerable<FrameEvent> ReadAsync(CancellationToken cancellation)
        => _call.ResponseStream.ReadAllAsync(cancellation);

    /// <summary>
    /// Hands one slot back, naming the pool it came from.
    /// The generation is echoed rather than assumed: a renegotiation re-announces the pool, and a release that
    /// crossed that announcement on the wire names a pool that is gone.
    /// The backend discards such a release instead of freeing a slot of the pool that replaced it.
    /// </summary>
    public Task ReleaseAsync(ulong generation, uint slot, ulong serial)
        => WriteAsync(new FramesRequest
        {
            Release = new FrameRelease { Generation = generation, Slot = slot, Serial = serial },
        });

    /// <summary>
    /// How many pixels this consumer will draw the frames at.
    /// A bound and not a size: the receive pipeline's scaler fixates inside it and corrects the pixel aspect
    /// ratio, so a tile smaller than its stream has the conversion done at its own size and a larger one gets the
    /// stream's own size rather than an upscale nobody asked for.
    /// Zero in either dimension leaves the pipeline where it is.
    /// </summary>
    public Task RenderSizeAsync(int width, int height)
        => WriteAsync(new FramesRequest
        {
            RenderSize = new FrameRenderSize { Width = width, Height = height },
        });

    /// <summary>
    /// Ends the subscription.
    /// The backend frees the pool as the call ends, so every handle this side imported names nothing afterwards:
    /// whoever made those imports disposes them before this runs.
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
            // A broken call is an ended call, which is what this method was asked for.
            // The dispose below is what releases it.
        }

        _call.Dispose();
        _writing.Dispose();
    }

    /// <summary>
    /// One write, serialized against the others.
    /// A failed write is swallowed: each is a message about a stream that has ended, a release of a slot nobody
    /// holds or a size for a pipeline that is gone.
    /// The reader is where the end is learned and reported, so raising here would report it twice, from the side
    /// that knows less.
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
