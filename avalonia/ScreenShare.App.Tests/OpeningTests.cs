using ScreenShare.Api.V1;
using ScreenShare.App.Features.Shell.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Where the window opens: the live surface while a stream is on the air, the viewer otherwise.
/// </summary>
public sealed class OpeningTests
{
    [Fact]
    public void ALiveStreamOpensOnBroadcast()
    {
        var live = new PublishState
        {
            Live = new PublishState.Types.Live { Publish = new PublishSettings(), StreamName = "lab04" },
        };

        Assert.Equal(Destination.Broadcast, Opening.For(live));
    }

    [Fact]
    public void AnythingElseOpensOnTheViewer()
    {
        Assert.Equal(Destination.Viewer, Opening.For(new PublishState()));
        Assert.Equal(Destination.Viewer, Opening.For(null));
    }
}
