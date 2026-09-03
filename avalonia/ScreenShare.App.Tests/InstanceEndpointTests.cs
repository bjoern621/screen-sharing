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

        var installed = OperatingSystem.IsWindows() ? @"\\.\pipe\screenshare-control-v1" : "control-v1.sock";
        Assert.EndsWith(installed, ControlEndpoint.Describe());
    }

    /// <summary>
    /// A named instance is a second address, so a build under development and an installed one both bind.
    /// </summary>
    [Fact]
    public void TheEndpointSeparatesANamedInstanceFromTheInstalledOne()
    {
        Environment.SetEnvironmentVariable(ControlEndpoint.EnvInstance, "dev");

        var named = OperatingSystem.IsWindows() ? @"\\.\pipe\screenshare-control-v1-dev" : "control-v1-dev.sock";
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
}
