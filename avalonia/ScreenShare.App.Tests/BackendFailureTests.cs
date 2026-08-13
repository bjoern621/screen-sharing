using Grpc.Core;
using ScreenShare.App.Backend;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// What a failed call turns into on screen.
///
/// <b>The division is who wrote the status, not which code it carries.</b>
/// The contract's table gives the backend <c>UNAVAILABLE</c> for a relay it could not reach and for a child
/// process that would not start (<c>docs/ipc-api.md</c>, "Errors"), so a shell reading that code as "nothing
/// is listening" answered a press of Start sharing with a sentence about the connection to a backend it had
/// just resolved a form through.
/// Prose the backend wrote reaches the reader intact, and only a failure the client made itself names the
/// endpoint.
/// </summary>
public sealed class BackendFailureTests
{
    /// <summary>
    /// The sentence naming what went wrong is the useful part of the screen, and the code it travelled under
    /// is not a second opinion about it.
    /// </summary>
    [Fact]
    public void ARefusalTheBackendServedKeepsItsOwnSentence()
    {
        const string served = "cannot start publishing: the relay at 192.168.1.9:8890 refused the connection";

        var failure = ControlBackend.Translate(
            new RpcException(new Status(StatusCode.Unavailable, served)), CancellationToken.None);

        var unavailable = Assert.IsType<BackendUnavailableException>(failure);
        Assert.Equal(served, unavailable.Message);
    }

    /// <summary>
    /// A failure the client library built from a local exception means the backend is not running, whatever
    /// code it wears: an absent named pipe arrives as <c>INTERNAL</c> on Windows, an unbound socket as
    /// <c>UNAVAILABLE</c>.
    /// The sentence names the address that was tried, that being the part a reader can act on.
    /// </summary>
    [Theory]
    [InlineData(StatusCode.Internal)]
    [InlineData(StatusCode.Unavailable)]
    public void AConnectionThatWasNeverMadeNamesTheEndpoint(StatusCode code)
    {
        var local = new Status(code, "Error starting gRPC call.", new IOException("the pipe is not there"));

        var failure = ControlBackend.Translate(new RpcException(local), CancellationToken.None);

        var unavailable = Assert.IsType<BackendUnavailableException>(failure);
        Assert.Contains("The backend is not running", unavailable.Message);
        Assert.Contains(ControlEndpoint.Describe(), unavailable.Message);
    }

    /// <summary>
    /// The exception promises a sentence and the screen asserts on it, so a status carrying no prose is
    /// named by its code rather than handed upwards blank.
    /// </summary>
    [Fact]
    public void AServedStatusWithNothingSaidNamesTheCode()
    {
        var failure = ControlBackend.Translate(
            new RpcException(new Status(StatusCode.Unknown, "")), CancellationToken.None);

        var unavailable = Assert.IsType<BackendUnavailableException>(failure);
        Assert.Contains("Unknown", unavailable.Message);
        Assert.Contains(ControlEndpoint.Describe(), unavailable.Message);
    }

    /// <summary>
    /// The flow cancels a resolve on every keystroke, and a superseded one is no failure the reader is told
    /// about.
    /// </summary>
    [Fact]
    public void AReadThisShellCancelledIsNotASentence()
    {
        using var cancelled = new CancellationTokenSource();
        cancelled.Cancel();

        var failure = ControlBackend.Translate(
            new RpcException(new Status(StatusCode.Cancelled, "Call canceled by the client.")), cancelled.Token);

        Assert.IsType<OperationCanceledException>(failure);
    }

    /// <summary>
    /// A <c>CANCELLED</c> the backend produced on its own is a failure like any other: nothing here asked
    /// for it.
    /// The token tells the two apart, and is checked alongside the code rather than instead of it.
    /// </summary>
    [Fact]
    public void ACancellationNobodyAskedForIsStillASentence()
    {
        const string served = "the run was cancelled while the encoder was starting";

        var failure = ControlBackend.Translate(
            new RpcException(new Status(StatusCode.Cancelled, served)), CancellationToken.None);

        var unavailable = Assert.IsType<BackendUnavailableException>(failure);
        Assert.Equal(served, unavailable.Message);
    }
}
