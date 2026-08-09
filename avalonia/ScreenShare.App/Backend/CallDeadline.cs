using Grpc.Core;
using Grpc.Core.Interceptors;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Backend;

/// <summary>
/// Puts a clock on every call that arrives without one.
///
/// <b>What it is for.</b> A control that waits on a round trip is unpressable while it waits
/// (<see cref="Mvvm.PendingCommand"/>), so an answer that never comes is not a slow button but
/// a dead one: the spinner is the only state left and nothing the reader can do moves it. That
/// is the wrong failure for a local socket, where the backend is a process on this machine and
/// "no answer" means it died, wedged, or lost the connection under the call - all of which are
/// facts worth showing rather than waiting through.
///
/// <b>Why here rather than at each call.</b> The bound is a property of talking to a backend
/// over a socket, not of any one method, and a per-call-site deadline is a rule that holds only
/// where somebody remembered it - the viewer's toggles were exactly the sites that had forgotten,
/// which is what made a lost answer strand them.
///
/// <b>Unary only.</b> The event stream and the frame channel are open for as long as the window
/// is, so a deadline on those would be a clock on the window itself. They are streaming calls
/// and this overrides nothing they go through.
///
/// A call that named its own deadline keeps it. The handshake does, and a caller that knows its
/// own method is slower than this bound says so rather than being overruled by it.
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
    /// The same bound on the blocking overload, which the generated client offers for every
    /// unary method. Nothing here calls one today, and that is exactly why it is covered: a
    /// clock that holds only on the overloads somebody remembered is the rule this type
    /// exists to replace, one level down.
    /// </summary>
    public override TResponse BlockingUnaryCall<TRequest, TResponse>(
        TRequest request,
        ClientInterceptorContext<TRequest, TResponse> context,
        BlockingUnaryCallContinuation<TRequest, TResponse> continuation)
        => continuation(request, Bound(context));

    /// <summary>
    /// The call's context with this bound on it, and the caller's own context untouched
    /// where it named a deadline of its own.
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
