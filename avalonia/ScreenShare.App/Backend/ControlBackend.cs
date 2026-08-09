using Grpc.Core;
using Grpc.Core.Interceptors;
using Grpc.Net.Client;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Backend;

/// <summary>
/// The control plane, over the local socket. This is the real thing behind
/// <see cref="IBackend"/>: every value, label, greying and sentence the setup flow draws is
/// computed by the Go backend and read through here.
///
/// <b>It evaluates nothing.</b> There is no codec name in this file, no encoder family, no
/// table and no rule - a greyed option arrives greyed, carrying the sentence that says why
/// (<c>docs/ipc-api.md</c>, "The rule"). What it owns is the three things a transport owns:
/// the channel, the handshake in front of it, and the translation of a failure into something
/// the caller above can act on.
///
/// <b>Hello runs before any other method.</b> It settles the contract major, so a shell built
/// against a major this backend does not implement learns it as a sentence naming both
/// numbers rather than as fields that silently arrive empty. It is run once and re-run after
/// a failure, which is what lets a window that opened before the backend did reach it later:
/// nothing here caches a dead connection, and <see cref="ControlEndpoint.ConnectAsync"/> is
/// called again by the channel whenever it needs one.
///
/// <b>The encoder probe is asked for once, and it is why this class has an event.</b>
/// <c>ResolveForm</c> reads what has been probed rather than probing, because a resolve is
/// called on every keystroke and the probe costs seconds; on a machine nothing has probed
/// yet, no codec is greyed for missing hardware. That is the honest reading of a fact nobody
/// has established - and it is also a form that would go on offering QSV on a machine with no
/// Intel GPU, so somebody has to ask. This does, once, in the background, and raises
/// <see cref="Changed"/> when the answer lands so the screen re-reads. Everything the probe
/// decides is still the backend's: what arrives here is only the news that the answer moved.
/// </summary>
public sealed class ControlBackend : IBackend
{
    /// <summary>
    /// The contract major this shell was generated against, matching Go's
    /// <c>control.ProtocolMajor</c>. It is sent on every handshake and the backend refuses a
    /// major it does not implement, so the two cannot quietly diverge.
    /// </summary>
    private const uint ProtocolMajor = 1;

    /// <summary>What this shell calls itself in the backend's log. It carries no behaviour.</summary>
    private const string ClientName = "avalonia";

    /// <summary>
    /// Bounds the handshake, and it is shorter than the bound every other call gets because
    /// it is the one call that answers off state the backend already holds. A backend that
    /// accepted the connection and did not answer this is not one worth waiting on.
    /// </summary>
    private static readonly TimeSpan HandshakeDeadline = TimeSpan.FromSeconds(5);

    /// <summary>
    /// Bounds every other call, through <see cref="CallDeadline"/>, and the number is the
    /// slowest honest effect rather than a guess: taking a decode down waits on the pipeline
    /// reaching NULL, which the receive package gives two attempts of three seconds each.
    /// Everything else on this contract answers in well under that.
    /// </summary>
    private static readonly TimeSpan CallDeadlineSpan = TimeSpan.FromSeconds(20);

    /// <summary>
    /// The probe's own bound, which is longer because it is the one method that does real
    /// work per call: it test-encodes on every engine this machine has. It is stated here
    /// rather than left to the general bound above, which it would exceed on a machine with
    /// several GPUs - and a probe that timed out is asked for again by the next read
    /// (<see cref="GreetAsync"/>), so the general bound would mean re-running it for as long
    /// as the window is open and never finishing one.
    /// </summary>
    private static readonly TimeSpan ProbeDeadline = TimeSpan.FromMinutes(3);

    private readonly ControlService.ControlServiceClient _client;

    /// <summary>The frame channel's client, on the same connection as the control one.</summary>
    private readonly FrameService.FrameServiceClient _frames;

    /// <summary>
    /// Guards the handshake, so several reads starting at once produce one <c>Hello</c> rather
    /// than one each.
    /// </summary>
    private readonly Lock _gate = new();

    /// <summary>
    /// The handshake, kept so it is awaited rather than repeated. A faulted one is dropped and
    /// started again on the next read, which is the whole of the reconnect: the failure was
    /// the backend not being there, and it may be there now.
    /// </summary>
    private Task? _handshake;

    /// <summary>
    /// Whether the probe has been asked for. One per instance, since the backend caches the
    /// result for its own process lifetime and a second request would be a second wait for an
    /// answer already given.
    /// </summary>
    private bool _probeAsked;

    public ControlBackend()
    {
        // Address and handler both matter. gRPC needs an origin to put on the request, and
        // over a pipe or a Unix socket there is no host to name, so the address is a
        // placeholder and the connect callback is what actually decides where the bytes go.
        // The channel is held by the client it is handed to and outlives this constructor
        // through it, which is the whole of its lifecycle: it is the window's connection and
        // the window is the process.
        var channel = GrpcChannel.ForAddress("http://localhost", new GrpcChannelOptions
        {
            HttpHandler = new SocketsHttpHandler
            {
                ConnectCallback = async (_, cancellation) => await ControlEndpoint.ConnectAsync(cancellation).ConfigureAwait(false),
                // The event stream is one long-lived call and a resolve is a short one; without
                // this they would share a connection and a resolve would queue behind whatever
                // the stream is doing.
                EnableMultipleHttp2Connections = true,
            },
        });

        // The control client goes through the deadline interceptor and the frame client does
        // not, which is the division that type is written around: everything on the control
        // service is a question with an answer, and the frame service is one call that stays
        // open for as long as a tile is drawn.
        _client = new ControlService.ControlServiceClient(channel.Intercept(new CallDeadline(CallDeadlineSpan)));
        // The frame channel is the second service on that same connection. One channel for
        // both is what the contract's own reasoning asks for: riding the same socket is what
        // avoids reinventing framing, versioning and cancellation for a stream of handle
        // metadata, and a second connection would have to be discovered and torn down
        // separately from the one the handshake already settled.
        _frames = new FrameService.FrameServiceClient(channel);
    }

    /// <inheritdoc />
    public event Action? Changed;

    /// <inheritdoc />
    public async Task<Settings> SettingsAsync(CancellationToken cancellation = default)
    {
        var answer = await ReadAsync(
            c => c.GetSettingsAsync(new GetSettingsRequest(), cancellationToken: cancellation), cancellation)
            .ConfigureAwait(false);

        return Assert.NotNull(answer.Settings, "the backend answers a settings read with settings");
    }

    /// <inheritdoc />
    public async Task<Catalog> CatalogAsync(CancellationToken cancellation = default)
    {
        var answer = await ReadAsync(
            c => c.GetCatalogAsync(new GetCatalogRequest(), cancellationToken: cancellation), cancellation)
            .ConfigureAwait(false);

        return Assert.NotNull(answer.Catalog, "the backend answers a catalog read with a catalog");
    }

    /// <inheritdoc />
    public Task<Form> ResolveFormAsync(Settings draft, CancellationToken cancellation = default)
    {
        Assert.NotNull(draft, "resolving a form needs the draft it is resolved against");

        return ReadAsync(
            c => c.ResolveFormAsync(new ResolveFormRequest { Settings = draft }, cancellationToken: cancellation),
            r => r.Form,
            cancellation);
    }

    /// <inheritdoc />
    public Task<PublishState> PublishStateAsync(CancellationToken cancellation = default)
        => ReadAsync(c => c.GetPublishStateAsync(new GetPublishStateRequest(), cancellationToken: cancellation), cancellation);

    /// <inheritdoc />
    public Task<RelayStatus> RelayStatusAsync(CancellationToken cancellation = default)
        => ReadAsync(c => c.GetRelayStatusAsync(new GetRelayStatusRequest(), cancellationToken: cancellation), cancellation);

    /// <inheritdoc />
    public async Task<IReadOnlyList<WatchKey>> WatchingAsync(CancellationToken cancellation = default)
    {
        var answer = await ReadAsync(
            c => c.GetViewerStateAsync(new GetViewerStateRequest(), cancellationToken: cancellation), cancellation)
            .ConfigureAwait(false);

        return answer.Viewers;
    }

    /// <inheritdoc />
    public async Task<IReadOnlyList<ReceiveStream>> ReceivingAsync(CancellationToken cancellation = default)
    {
        var answer = await ReadAsync(
            c => c.GetReceiveStateAsync(new GetReceiveStateRequest(), cancellationToken: cancellation), cancellation)
            .ConfigureAwait(false);

        return answer.Streams;
    }

    /// <inheritdoc />
    public Task StartPublishAsync(Settings settings, CancellationToken cancellation = default)
    {
        Assert.NotNull(settings, "starting a publish names the settings the encoder runs on");

        return ReadAsync(
            c => c.StartPublishAsync(new StartPublishRequest { Settings = settings }, cancellationToken: cancellation),
            cancellation);
    }

    /// <inheritdoc />
    public Task StopPublishAsync(CancellationToken cancellation = default)
        => ReadAsync(c => c.StopPublishAsync(new StopPublishRequest(), cancellationToken: cancellation), cancellation);

    /// <inheritdoc />
    public Task<double> MeasureUplinkAsync(CancellationToken cancellation = default)
        => ReadAsync(
            c => c.MeasureUplinkAsync(new MeasureUplinkRequest(), cancellationToken: cancellation),
            r => r.Mbps,
            cancellation);

    /// <inheritdoc />
    public Task StartWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
        => KeyedAsync(streamName, transport, "opening a viewer",
            (c, key) => c.StartWatchAsync(new StartWatchRequest { Viewer = key }, cancellationToken: cancellation),
            cancellation);

    /// <inheritdoc />
    public Task StopWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
        => KeyedAsync(streamName, transport, "closing a viewer",
            (c, key) => c.StopWatchAsync(new StopWatchRequest { Viewer = key }, cancellationToken: cancellation),
            cancellation);

    /// <inheritdoc />
    public Task StartReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        => KeyedAsync(streamName, transport, "opening a decode",
            (c, key) => c.StartReceiveAsync(new StartReceiveRequest { Stream = key }, cancellationToken: cancellation),
            cancellation);

    /// <inheritdoc />
    public Task StopReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        => KeyedAsync(streamName, transport, "closing a decode",
            (c, key) => c.StopReceiveAsync(new StopReceiveRequest { Stream = key }, cancellationToken: cancellation),
            cancellation);

    /// <inheritdoc />
    public async Task<FrameChannel> OpenFramesAsync(string streamName, string transport, CancellationToken cancellation = default)
    {
        // The handshake first, like every other call: a frame channel opened before the
        // contract major was settled would be a stream of handles agreed on by two sides that
        // never established they mean the same thing by one.
        await GreetAsync().ConfigureAwait(false);

        try
        {
            return await FrameChannel.OpenAsync(_frames, streamName, transport, cancellation).ConfigureAwait(false);
        }
        catch (RpcException e)
        {
            throw Translate(e, cancellation);
        }
    }

    /// <inheritdoc />
    public Task OpenLogAsync(string path, CancellationToken cancellation = default)
    {
        Assert.That(path.Length > 0, "opening a run log names the log the backend handed out");

        return ReadAsync(c => c.OpenLogAsync(new OpenLogRequest { Path = path }, cancellationToken: cancellation), cancellation);
    }

    /// <inheritdoc />
    public Task OpenLogsFolderAsync(CancellationToken cancellation = default)
        => ReadAsync(c => c.OpenLogsFolderAsync(new OpenLogsFolderRequest(), cancellationToken: cancellation), cancellation);

    /// <inheritdoc />
    public async IAsyncEnumerable<Event> SubscribeAsync(
        [System.Runtime.CompilerServices.EnumeratorCancellation] CancellationToken cancellation = default)
    {
        await GreetAsync().ConfigureAwait(false);

        // The call is opened outside the loop so a failure to open it is translated the same
        // way a failed read is, rather than surfacing as a status from inside an enumeration
        // the caller is already iterating.
        using var call = _client.Subscribe(new SubscribeRequest(), cancellationToken: cancellation);

        while (true)
        {
            bool more;
            try
            {
                more = await call.ResponseStream.MoveNext(cancellation).ConfigureAwait(false);
            }
            catch (RpcException e)
            {
                throw Translate(e, cancellation);
            }

            if (!more)
            {
                yield break;
            }

            yield return call.ResponseStream.Current;
        }
    }

    /// <summary>
    /// One call, with the handshake in front of it and the failure translated behind it.
    ///
    /// Every read and every effect is the same three steps, so they are written once. The
    /// delegate takes the client rather than closing over it so the shape reads as what it
    /// is: this method decides nothing about which call is made.
    /// </summary>
    private async Task<T> ReadAsync<T>(
        Func<ControlService.ControlServiceClient, AsyncUnaryCall<T>> call, CancellationToken cancellation)
    {
        await GreetAsync().ConfigureAwait(false);

        try
        {
            return await call(_client).ResponseAsync.ConfigureAwait(false);
        }
        catch (RpcException e)
        {
            throw Translate(e, cancellation);
        }
    }

    /// <summary>
    /// One effect keyed by the pair a viewer and a decode are identified by.
    ///
    /// The four methods above differ in the request they wrap that pair in and in nothing
    /// else, so the pair is asserted and built here. <paramref name="what"/> is the effect
    /// named as its assertions name it, which is the only part of the sentence that is the
    /// caller's; the invariant itself is one sentence because it is one invariant.
    /// </summary>
    private Task KeyedAsync<TResponse>(
        string streamName,
        string transport,
        string what,
        Func<ControlService.ControlServiceClient, WatchKey, AsyncUnaryCall<TResponse>> call,
        CancellationToken cancellation)
    {
        Assert.That(streamName.Length > 0, $"{what} names the stream it is for");
        Assert.That(transport.Length > 0, $"{what} names the leg it runs over", streamName);

        var key = new WatchKey { StreamName = streamName, Transport = transport };
        return ReadAsync(c => call(c, key), cancellation);
    }

    /// <summary>
    /// The same read, for a method whose response wraps the state rather than being it.
    ///
    /// The contract wraps every read whose answer does not also travel on the event stream,
    /// so that a response can grow a field the state itself has no business carrying. The
    /// unwrapping belongs here and nowhere else: a view model that reached through the
    /// envelope would be a second place that knows which reads are wrapped.
    /// </summary>
    private async Task<TState> ReadAsync<TResponse, TState>(
        Func<ControlService.ControlServiceClient, AsyncUnaryCall<TResponse>> call,
        Func<TResponse, TState> state,
        CancellationToken cancellation)
        => state(await ReadAsync(call, cancellation).ConfigureAwait(false));

    /// <summary>
    /// Settles the contract version, once, and asks for the encoder probe behind it.
    ///
    /// The handshake is shared rather than per call, so two reads racing produce one
    /// <c>Hello</c>. It is deliberately not given the caller's token: one caller abandoning
    /// its read would otherwise cancel the handshake every other caller is waiting on.
    ///
    /// The probe is asked for here rather than from inside the handshake, and that is what
    /// makes "a probe that failed is not remembered as done" true. A settled handshake is
    /// kept and never re-run, so a flag only the handshake consults is a flag nothing reads
    /// again; asked for on every read, the reset takes effect on the next one.
    /// <see cref="Probe"/> is idempotent, so every read after the first asks for a state
    /// that already holds and does nothing.
    /// </summary>
    private async Task GreetAsync()
    {
        Task handshake;
        lock (_gate)
        {
            if (_handshake is null || _handshake.IsFaulted || _handshake.IsCanceled)
            {
                _handshake = HelloAsync();
            }

            handshake = _handshake;
        }

        await handshake.ConfigureAwait(false);
        Probe();
    }

    private async Task HelloAsync()
    {
        HelloResponse hello;
        try
        {
            hello = await _client.HelloAsync(
                new HelloRequest { Client = ClientName, ProtocolMajor = ProtocolMajor },
                deadline: DateTime.UtcNow.Add(HandshakeDeadline)).ResponseAsync.ConfigureAwait(false);
        }
        catch (RpcException e)
        {
            throw Translate(e, CancellationToken.None);
        }

        Assert.That(
            hello.ProtocolMajor == ProtocolMajor,
            "a settled handshake leaves both sides on one contract major", ProtocolMajor, hello.ProtocolMajor);
    }

    /// <summary>
    /// Asks the backend to run the encoder probe, once, without waiting for it.
    ///
    /// Not awaited by the handshake, and that is the point: the probe test-encodes on every
    /// engine and takes seconds, so a read that waited for it would put those seconds in front
    /// of the window's first form. The screen paints what is known now - which greys nothing
    /// for missing hardware - and <see cref="Changed"/> tells it to read again when the answer
    /// lands.
    ///
    /// A probe that failed is not remembered as done. The backend was unreachable, so nothing
    /// was probed and the next read asks again (<see cref="GreetAsync"/>).
    /// </summary>
    private void Probe()
    {
        lock (_gate)
        {
            if (_probeAsked)
            {
                return;
            }

            _probeAsked = true;
        }

        _ = ProbeAsync();
    }

    private async Task ProbeAsync()
    {
        try
        {
            await _client.ProbeEncodersAsync(
                new ProbeEncodersRequest(), deadline: DateTime.UtcNow.Add(ProbeDeadline))
                .ResponseAsync.ConfigureAwait(false);
        }
        catch (RpcException)
        {
            lock (_gate)
            {
                _probeAsked = false;
            }

            return;
        }

        // Raised on whichever thread the call completed on, which is why the contract states
        // it: a subscriber that writes a bound property marshals this back itself.
        Changed?.Invoke();
    }

    /// <summary>
    /// Turns a failed call into what the caller above is written against: a read this shell
    /// abandoned, or a sentence the reader sees.
    ///
    /// The two are not gRPC's own division, which is why the translation is here and not left
    /// to a <c>catch</c> upstairs. A superseded resolve is nobody's business - the flow
    /// cancels one on every keystroke - so it becomes an
    /// <see cref="OperationCanceledException"/>, and the token is checked alongside the code so
    /// a <c>CANCELLED</c> the backend produced on its own is not mistaken for one this shell
    /// asked for.
    ///
    /// <b>Everything else divides by who wrote the status, not by which code it carries.</b>
    /// A status the backend produced carries prose written for a person and is the screen's to
    /// show verbatim: a relay that could not be reached and a child process that would not
    /// start are both <c>UNAVAILABLE</c> in the contract's table
    /// (<c>docs/ipc-api.md</c>, "Errors"), and reading that code as "nothing is listening"
    /// would answer a press of Go live with a sentence about the connection it just used.
    /// The one thing this side may add is the address, and only where the connection is what
    /// failed.
    ///
    /// Which side wrote it is what <see cref="Status.DebugException"/> answers. The client
    /// library sets it on a status it made from a local failure and leaves it null on one that
    /// arrived from the server, so a connect that was refused is told apart from a refusal
    /// that was served - and it is told apart by code rather than by matching on a sentence,
    /// which is the input that changes without anything failing to compile.
    ///
    /// A local failure is the backend not running, whatever code it wears: an absent named
    /// pipe arrives as <c>INTERNAL</c> on Windows with nothing said, an unbound socket as
    /// <c>UNAVAILABLE</c>. Both name the address that was tried, because "nothing is listening
    /// on this" is the fact and the path is what makes it actionable.
    ///
    /// A served status with no prose on it is left: the exception promises a sentence, so the
    /// code it failed with is named rather than handed upwards empty - a screen whose failure
    /// text is blank says less than one naming a status, and the assertion upstairs is right
    /// to refuse it.
    /// </summary>
    internal static Exception Translate(RpcException e, CancellationToken cancellation)
    {
        if (e.StatusCode == StatusCode.Cancelled && cancellation.IsCancellationRequested)
        {
            return new OperationCanceledException(cancellation);
        }

        // A deadline is this side's own clock running out, so it is named before the check
        // below: the status was written here and carries no prose, and the general wording
        // for that says the backend answered - which is the one thing that did not happen.
        //
        // What it says about the attempt is what a reader has to decide with, and the honest
        // answer is that nobody here knows: the call may have been done and its answer lost.
        // Saying so is only useful because trying again is safe for an effect that names a
        // state - a repeat of one that landed is a call that finds its work already done.
        // The sentence says "names a state" rather than "every call", because ApplyToStream
        // names a transition on purpose and a second one of those is a second restart.
        if (e.StatusCode == StatusCode.DeadlineExceeded)
        {
            return new BackendUnavailableException(
                $"The backend did not answer in time over {ControlEndpoint.Describe()}. "
                + "Whether it acted before going quiet is not visible from here, "
                + "so anything that names a state - starting or stopping a stream, a viewer or a decode - "
                + "is safe to ask for again.", e);
        }

        if (e.Status.DebugException is not null)
        {
            return new BackendUnavailableException(
                $"The backend is not running: nothing is listening on {ControlEndpoint.Describe()} ({e.StatusCode}).", e);
        }

        var detail = e.Status.Detail;
        if (string.IsNullOrWhiteSpace(detail))
        {
            detail = $"The backend answered {e.StatusCode} over {ControlEndpoint.Describe()} and said nothing further.";
        }

        return new BackendUnavailableException(detail, e);
    }
}
