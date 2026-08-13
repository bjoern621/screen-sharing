using Grpc.Core;
using Grpc.Core.Interceptors;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Backend;

/// <summary>
/// Deadline on every unary call that arrives without one.
///
/// A control waiting on a round trip is unpressable while it waits (<see cref="Mvvm.PendingCommand"/>), so an
/// answer that never comes leaves a dead button rather than a slow one.
/// Over a local socket, no answer means the backend died, wedged, or lost the connection under the call, and
/// each of those is worth showing rather than waiting through.
///
/// The bound belongs to talking to a backend over a socket rather than to any one method, so a per-call-site
/// deadline holds only where somebody remembered it.
///
/// Unary only.
/// The event stream and the frame channel stay open for as long as the window does, and nothing here
/// overrides what a streaming call goes through.
///
/// A call that named its own deadline keeps it: the handshake and the encoder probe both do.
/// </summary>
public sealed class CallDeadline : Interceptor
{
    private readonly TimeSpan _within;

    public CallDeadline(TimeSpan within)
    {
        Assert.That(within > TimeSpan.Zero, "a deadline is a length of time");
        _within = within;
    }

    public override AsyncUnaryCall<TResponse> AsyncUnaryCall<TRequest, TResponse>(
        TRequest request,
        ClientInterceptorContext<TRequest, TResponse> context,
        AsyncUnaryCallContinuation<TRequest, TResponse> continuation)
        => continuation(request, Bound(context));

    /// <summary>
    /// The same bound on the blocking overload the generated client offers, so which overload a call site
    /// picks does not decide whether there is a clock on it.
    /// </summary>
    public override TResponse BlockingUnaryCall<TRequest, TResponse>(
        TRequest request,
        ClientInterceptorContext<TRequest, TResponse> context,
        BlockingUnaryCallContinuation<TRequest, TResponse> continuation)
        => continuation(request, Bound(context));

    /// <summary>
    /// The call's context with this bound on it, or the caller's own untouched where it named a deadline.
    /// </summary>
    private ClientInterceptorContext<TRequest, TResponse> Bound<TRequest, TResponse>(
        ClientInterceptorContext<TRequest, TResponse> context)
        where TRequest : class
        where TResponse : class
        => context.Options.Deadline is not null
            ? context
            : new ClientInterceptorContext<TRequest, TResponse>(
                context.Method,
                context.Host,
                context.Options.WithDeadline(DateTime.UtcNow.Add(_within)));
}
