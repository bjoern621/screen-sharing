namespace ScreenShare.App.Backend;

/// <summary>
/// The backend could not describe the screen.
/// Its <see cref="Exception.Message"/> is prose written for a person and is shown as it stands.
///
/// It exists so the layer above stays free of the transport.
/// A view model that caught an <c>RpcException</c> would be a view model that knows the calls are gRPC, which
/// is one more thing than "ask the backend and draw the answer" - and the two failures it has to tell apart
/// are not gRPC's division anyway.
/// A read this shell abandoned is an <see cref="OperationCanceledException"/> and is nobody's business;
/// everything else is this, and it is a sentence the reader sees.
///
/// <b>Neither side owes the other a compatibility shim</b> (<c>docs/ipc-api.md</c>, "What each side owes").
/// A shell without a backend says so and offers to look again; it does not invent a form to draw in the
/// meantime, because a control the shell made up is a lie on screen where a stated absence is a fact.
/// </summary>
public sealed class BackendUnavailableException(string message, Exception? inner = null)
    : Exception(message, inner);
