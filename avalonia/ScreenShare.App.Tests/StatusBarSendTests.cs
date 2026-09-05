using ScreenShare.App.Backend;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.StatusBar.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The send-logs button on the status band: one press bundles the machine's logs and facts
/// at the backend and shows what came back.
///
/// Defect locked out: a report that went out with nothing on screen saying so,
/// and a refusal a reader cannot select and carry to the operator.
/// </summary>
public sealed class StatusBarSendTests
{
    private static StatusBarViewModel Band(Func<CancellationToken, Task<string>> send)
    {
        var band = new StatusBarViewModel(send, action => action());
        band.Show(Destination.Setup, "", [], "", "0.4.0");
        return band;
    }

    /// <summary>Polls, the command completing off the press.</summary>
    private static async Task Eventually(Func<bool> landed)
    {
        for (var i = 0; i < 200 && !landed(); i++)
        {
            await Task.Delay(10);
        }

        Assert.True(landed());
    }

    /// <summary>
    /// The answer names the stored bundle, which is what the reader quotes to the operator.
    /// </summary>
    [Fact]
    public async Task ASentReportShowsTheNameItLandedUnder()
    {
        var band = Band(_ => Task.FromResult("20260905-013000-ab12cd34.tar.gz"));

        band.SendLogs.Execute(null);
        await Eventually(() => band.ShowsSendOutcome);

        Assert.Contains("20260905-013000-ab12cd34.tar.gz", band.SendOutcome);
        Assert.False(band.SendFailed);
    }

    /// <summary>
    /// A refusal is the backend's sentence about the attempt and shows as it stands
    /// (<c>docs/ipc-api.md</c>, "Errors").
    /// </summary>
    [Fact]
    public async Task ARefusalShowsTheBackendsSentence()
    {
        var band = Band(_ => throw new BackendUnavailableException("cannot send the report: the group service at https://relay.example cannot be reached"));

        band.SendLogs.Execute(null);
        await Eventually(() => band.ShowsSendOutcome);

        Assert.Contains("cannot be reached", band.SendOutcome);
        Assert.True(band.SendFailed);
    }

    /// <summary>
    /// The outcome answers the band's own effect rather than a destination,
    /// so a render pass for another screen leaves it standing.
    /// </summary>
    [Fact]
    public async Task TheOutcomeSurvivesADestinationChange()
    {
        var band = Band(_ => Task.FromResult("report-1"));

        band.SendLogs.Execute(null);
        await Eventually(() => band.ShowsSendOutcome);
        band.Show(Destination.Viewer, "", [], "", "0.4.0");

        Assert.True(band.ShowsSendOutcome);
    }
}
