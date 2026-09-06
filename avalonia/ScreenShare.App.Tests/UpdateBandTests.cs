using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Shell.Update.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The version in the status band, which is the control that asks for a release check,
/// and the line beside it saying what the check found.
///
/// Defects locked out: a version offering a check on an install whose files belong to a package
/// manager, a restart offered before anything is downloaded, and a failure a reader cannot select.
/// </summary>
public sealed class UpdateBandTests
{
    private static UpdateViewModel Band(UpdateState state)
    {
        var backend = new SeededBackend("linux") { Update = state };
        var updates = Flows.Updates(backend);
        updates.Apply();
        return updates;
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
    /// An install that asks the release service nothing offers no way to ask.
    /// The tip says why rather than leaving a dead control: it is the whole of what a reader can act on.
    /// </summary>
    [Fact]
    public void AnInstallThatChecksNothingOffersNoCheck()
    {
        var band = Band(new UpdateState
        {
            Stage = UpdateStage.Off,
            Unchecked = new Text { Code = TextCode.UpdateCheckOff },
        });

        Assert.False(band.CanCheck);
        Assert.False(band.ShowsLine);
        Assert.NotEqual(Updates.Check, band.CheckHint);
        Assert.Contains("MIRRORME_UPDATE_CHECK", band.CheckHint);
    }

    /// <summary>
    /// A build at the published release says so and offers nothing to press:
    /// a line a reader can click is one with something behind it.
    /// </summary>
    [Fact]
    public void ABuildAtThePublishedReleaseSaysSoAndOpensNothing()
    {
        var band = Band(new UpdateState { Stage = UpdateStage.Current, Running = "0.5.0", Latest = "v0.5.0" });

        Assert.True(band.CanCheck);
        Assert.Equal(Updates.Current, band.Line);
        Assert.False(band.OpensDialog);
        Assert.True(band.ShowsPlainLine);
        Assert.False(band.CanInstall);
    }

    /// <summary>
    /// A staged release is the one state the restart is offered in.
    /// The dialog says what already happened, so the reader knows the press costs them a restart
    /// and no download.
    /// </summary>
    [Fact]
    public void AStagedReleaseOffersTheRestart()
    {
        var band = Band(new UpdateState
        {
            Stage = UpdateStage.Ready,
            Running = "0.4.0",
            Latest = "v0.5.0",
            Page = "https://example.invalid/releases/v0.5.0",
        });

        Assert.True(band.OpensDialog);
        Assert.True(band.Open.CanExecute(null));
        Assert.True(band.CanInstall);
        Assert.Contains("v0.5.0", band.Title);
        Assert.Equal(Updates.Restart, band.Body);
        Assert.True(band.HasPage);
    }

    /// <summary>
    /// An install a package manager owns names the release and refuses the restart,
    /// pointing the reader at what does update it.
    /// </summary>
    [Fact]
    public void AManagedInstallNamesTheReleaseAndNotTheRestart()
    {
        var band = Band(new UpdateState
        {
            Stage = UpdateStage.Available,
            Running = "0.4.0",
            Latest = "v0.5.0",
            Uninstallable = new Text
            {
                Code = TextCode.UpdateChannelManaged,
                Args = { new TextArg { Name = TextArgName.Channel, Id = "nix" } },
            },
        });

        Assert.True(band.OpensDialog);
        Assert.False(band.CanInstall);
        Assert.True(band.ShowsHeld);
        Assert.Contains("Nix", band.Held);
    }

    /// <summary>
    /// A failure is text rather than a control, and carries the failing side's own words:
    /// that string is what a reader selects and puts in a bug report.
    /// </summary>
    [Fact]
    public void AFailureIsSelectableTextCarryingWhatFailed()
    {
        var band = Band(new UpdateState
        {
            Stage = UpdateStage.Failed,
            Running = "0.4.0",
            Failure = new Text { Code = TextCode.UpdateServiceUnreadable },
            Detail = "dial tcp: lookup api.github.com: no such host",
        });

        Assert.True(band.IsFailure);
        Assert.True(band.ShowsPlainLine);
        Assert.False(band.OpensDialog);
        Assert.True(band.ShowsDetail);
        Assert.Contains("no such host", band.Detail);
    }

    /// <summary>
    /// The version's press reaches the backend, which is the whole of what the band decides:
    /// what the check finds arrives on the event stream, from whichever window asked.
    /// </summary>
    [Fact]
    public async Task PressingTheVersionAsksTheBackendToCheck()
    {
        var backend = new SeededBackend("linux")
        {
            Update = new UpdateState { Stage = UpdateStage.Unchecked, Running = "0.4.0" },
        };

        var band = Flows.Updates(backend);
        band.Apply();

        Assert.True(band.Check.CanExecute(null));
        band.Check.Execute(null);

        await Eventually(() => backend.UpdateChecks == 1);
    }

    /// <summary>
    /// The restart follows the reply rather than racing it:
    /// the applier waits for this process to exit, so the app closes once the call has been accepted.
    /// </summary>
    [Fact]
    public async Task TheRestartFollowsTheInstallCall()
    {
        var backend = new SeededBackend("linux")
        {
            Update = new UpdateState { Stage = UpdateStage.Ready, Running = "0.4.0", Latest = "v0.5.0" },
        };

        var band = Flows.Updates(backend);
        band.Apply();

        var closed = 0;
        band.RestartRequested += () => closed++;

        Assert.True(band.Install.CanExecute(null));
        band.Install.Execute(null);

        await Eventually(() => backend.UpdateInstalls == 1 && closed == 1);
    }
}
