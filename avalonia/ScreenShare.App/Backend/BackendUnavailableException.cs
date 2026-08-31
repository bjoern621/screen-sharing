namespace ScreenShare.App.Backend;

/// <summary>
/// Backend could not describe the screen.
/// <see cref="Exception.Message"/> is prose for a person, shown as it stands.
///
/// Keeps the transport out of the layer above: a view model catching an <c>RpcException</c> would know the calls
/// are gRPC, and the two failures it tells apart are not gRPC's own division.
/// A read this shell abandoned is an <see cref="OperationCanceledException"/> instead and reaches no screen.
///
/// Neither side owes the other a compatibility shim (<c>docs/ipc-api.md</c>, "What each side owes"), so a shell
/// without a backend says so and invents no form: a control the shell made up is a lie on screen where a stated
/// absence is a fact.
/// </summary>
public sealed class BackendUnavailableException(string message, Exception? inner = null)
    : Exception(message, inner);
