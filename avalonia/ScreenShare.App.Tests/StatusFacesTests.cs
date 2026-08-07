using ScreenShare.App.Relay;
using ScreenShare.App.Ui;
using Xunit;

namespace ScreenShare.App.Tests;

public sealed class StatusFacesTests
{
    private static readonly RelayStatus Reachable = RelayStatus.Live([]);
    private static readonly RelayStatus Unreachable = RelayStatus.Failed("connection refused");

    [Fact]
    public void BeforeTheFirstPollTheStatusIsIdle()
        => Assert.Equal(StatusKind.Idle, StatusFaces.KindOf(RelayStatus.Unknown, polling: false));

    [Fact]
    public void TheFirstPollInFlightIsTheOnlyConnectingState()
        => Assert.Equal(StatusKind.Connecting, StatusFaces.KindOf(RelayStatus.Unknown, polling: true));

    [Theory]
    [InlineData(true)]
    [InlineData(false)]
    public void ARefreshOfALiveRelayStaysLive(bool polling)
        => Assert.Equal(StatusKind.Live, StatusFaces.KindOf(Reachable, polling));

    [Theory]
    [InlineData(true)]
    [InlineData(false)]
    public void ARefreshOfADeadRelayStaysFailed(bool polling)
        => Assert.Equal(StatusKind.Failed, StatusFaces.KindOf(Unreachable, polling));

    [Fact]
    public void OnlyTheLiveFacePulsesAndOnlyTheConnectingOneSpins()
    {
        Assert.True(StatusFaces.Of(StatusKind.Live).Pulses);
        Assert.False(StatusFaces.Of(StatusKind.Failed).Pulses);
        Assert.True(StatusFaces.Of(StatusKind.Connecting).Spins);
        Assert.False(StatusFaces.Of(StatusKind.Live).Spins);
    }

    [Fact]
    public void RedNeverMeansLive()
    {
        Assert.Equal(StatusFaces.Primary, StatusFaces.Of(StatusKind.Live).Color);
        Assert.Equal(StatusFaces.Destructive, StatusFaces.Of(StatusKind.Failed).Color);
    }

    [Fact]
    public void EveryStatusKindHasAFace()
    {
        foreach (var kind in Enum.GetValues<StatusKind>())
        {
            Assert.NotNull(StatusFaces.Of(kind));
        }
    }
}
