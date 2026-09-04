using System.Diagnostics;
using ScreenShare.App.Backend;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Endpoint an instance serves, and the switch that leaves starting a backend to whoever started this shell.
///
/// One class, xunit running a class's tests in sequence and separate classes at once:
/// the environment these write is the process's, so a second class touching it would read the first one's value.
/// </summary>
public sealed class InstanceEndpointTests : IDisposable
{
    private readonly string? _instance = Environment.GetEnvironmentVariable(ControlEndpoint.EnvInstance);
    private readonly string? _spawn = Environment.GetEnvironmentVariable(BackendProcess.EnvSpawn);

    public void Dispose()
    {
        Environment.SetEnvironmentVariable(ControlEndpoint.EnvInstance, _instance);
        Environment.SetEnvironmentVariable(BackendProcess.EnvSpawn, _spawn);
    }

    /// <summary>Address every install answers on, which is the one an unset variable names.</summary>
    [Fact]
    public void TheEndpointIsTheInstalledOneWhereNoInstanceIsNamed()
    {
        Environment.SetEnvironmentVariable(ControlEndpoint.EnvInstance, null);

        var installed = OperatingSystem.IsWindows() ? @"\\.\pipe\mirrorme-control-v1" : "control-v1.sock";
        Assert.EndsWith(installed, ControlEndpoint.Describe());
    }

    /// <summary>
    /// A named instance is a second address, so a build under development and an installed one both bind.
    /// </summary>
    [Fact]
    public void TheEndpointSeparatesANamedInstanceFromTheInstalledOne()
    {
        Environment.SetEnvironmentVariable(ControlEndpoint.EnvInstance, "dev");

        var named = OperatingSystem.IsWindows() ? @"\\.\pipe\mirrorme-control-v1-dev" : "control-v1-dev.sock";
        Assert.EndsWith(named, ControlEndpoint.Describe());
    }

    /// <summary>
    /// Off, the shell reports the endpoint as unserved rather than reaching for whatever binary PATH holds.
    /// </summary>
    [Fact]
    public void NoBackendIsStartedWhereSpawningIsOff()
    {
        Environment.SetEnvironmentVariable(BackendProcess.EnvSpawn, "0");

        Assert.False(BackendProcess.EnsureStarted());
    }

    /// <summary>
    /// Nothing listening is learnt from the connect refusing, not from the caller's deadline running out:
    /// starting a backend hangs off that refusal (<see cref="ControlEndpoint.ConnectAsync"/>), and a connect
    /// that waits for the token leaves every install with no backend up reporting a timeout, with no backend
    /// started.
    /// A named pipe nobody serves is where the platforms differ: the pipe client waits for one to appear,
    /// where a socket with nothing bound is refused at once.
    /// </summary>
    [Fact]
    public async Task AnEndpointNobodyServesRefusesTheConnectBeforeTheCallerGivesUp()
    {
        Environment.SetEnvironmentVariable(BackendProcess.EnvSpawn, "0");
        Environment.SetEnvironmentVariable(ControlEndpoint.EnvInstance, "nobody-serves-" + Guid.NewGuid().ToString("N"));

        using var caller = new CancellationTokenSource(TimeSpan.FromSeconds(10));
        var watch = Stopwatch.StartNew();

        var refusal = await Assert.ThrowsAnyAsync<Exception>(async () => await ControlEndpoint.ConnectAsync(caller.Token));

        Assert.False(caller.IsCancellationRequested, $"the connect waited out the caller's token: {refusal}");
        Assert.IsNotType<OperationCanceledException>(refusal);
        Assert.True(watch.Elapsed < TimeSpan.FromSeconds(5), $"the refusal took {watch.Elapsed}: {refusal}");
    }
}
