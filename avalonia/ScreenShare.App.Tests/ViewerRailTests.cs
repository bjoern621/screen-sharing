using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Viewer.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A group is a path prefix, so every entry of a member's list carries the same one and it separates none of them.
/// What the rail prints and what it opens are two strings.
/// Nothing else would notice a row printing a prefix,
/// or a decode asked for on a name the relay has no path for.
/// </summary>
public sealed class ViewerRailTests
{
    /// <summary>Shape of an id, so a path reads as one a group service would have granted.</summary>
    private const string Prefix = "MFZWIZLTOQ2DGNBVGY3TQOJQGE/";

    private static (ViewerViewModel Viewer, SeededBackend Backend) Rail(RelayPath path)
    {
        var backend = new SeededBackend("linux")
        {
            Relay = new RelayStatus { Reachable = true, Paths = { path } },
        };

        var session = new Session(backend, static action => action());
        session.Start();

        var viewer = Flows.Viewer(backend, session);
        viewer.Apply();
        return (viewer, backend);
    }

    [Fact]
    public void AnEntryPrintsTheStreamsOwnNameInsideItsGroup()
    {
        var (viewer, _) = Rail(new RelayPath { Name = Prefix + "desk", OwnName = "desk", Ready = true });

        var row = Assert.Single(viewer.Streams);
        Assert.Equal("desk", row.Label);
        Assert.Equal(Prefix + "desk", row.Name);
    }

    /// <summary>The whole path is what the relay serves, so the shortened word never reaches an effect.</summary>
    [Fact]
    public void WatchingOneAsksForTheWholePath()
    {
        var (viewer, backend) = Rail(new RelayPath { Name = Prefix + "desk", OwnName = "desk", Ready = true });

        Assert.Single(viewer.Streams).Show.Execute(null);

        Assert.Equal(Prefix + "desk", Assert.Single(backend.Decoded).StreamName);
    }

    /// <summary>
    /// A backend older than the field names no own name,
    /// and a row drawing that answer as it stands is a list of dots with no words beside them.
    /// </summary>
    [Fact]
    public void ASnapshotNamingNoOwnNamePrintsTheWholePath()
    {
        var (viewer, _) = Rail(new RelayPath { Name = Prefix + "desk", Ready = true });

        Assert.Equal(Prefix + "desk", Assert.Single(viewer.Streams).Label);
    }

    /// <summary>
    /// A relay that authenticates nobody carries bare names,
    /// and the backend answers the whole name as the stream's own one.
    /// </summary>
    [Fact]
    public void APathUnderNoPrefixPrintsAsItArrived()
    {
        var (viewer, _) = Rail(new RelayPath { Name = "desk", OwnName = "desk", Ready = true });

        var row = Assert.Single(viewer.Streams);
        Assert.Equal("desk", row.Label);
        Assert.Equal("desk", row.Name);
    }
}
