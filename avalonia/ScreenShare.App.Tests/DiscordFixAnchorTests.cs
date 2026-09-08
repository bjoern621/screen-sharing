using ScreenShare.App.Backend;
using ScreenShare.App.Controls;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ViewModel;
using ScreenShare.App.Features.Shell.Settings.Model;
using ScreenShare.App.Features.Shell.Settings.ViewModel;
using ScreenShare.App.Features.Shell.Update.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A group that follows the voice channel needs a Discord link,
/// and the button making one stands in the settings dialog.
/// The check line saying so leads there, at the heading the button stands under.
/// The step drawing the group source is where the fault is reported and nowhere it can be fixed.
/// </summary>
public sealed class DiscordFixAnchorTests
{
    private static readonly Action<Action> Inline = action => action();

    /// <summary>The wizard and the dialog over one session and one draft, as the window builds them.</summary>
    private static async Task<(SetupViewModel Flow, AppSettingsViewModel Settings)> WindowAsync(
        SeededBackend backend)
    {
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        var updates = new UpdateViewModel(backend, session, Inline);
        var settings = new AppSettingsViewModel(backend, form, session, updates, Inline);
        var flow = new SetupViewModel(
            backend, form, session, Flows.Picker(backend, form, session), settings.OpenAtDiscordCommand, Inline);

        await flow.Settled;
        return (flow, settings);
    }

    /// <summary>Follows the voice channel with no account linked, which is what the backend blocks on.</summary>
    private static SeededBackend Unlinked() => new("linux") { FollowDiscord = true };

    [Fact]
    public async Task TheLineAboutTheLinkLeadsToTheSettingsDialog()
    {
        var (flow, settings) = await WindowAsync(Unlinked());

        var blocking = flow.Rail.Checks.Single(check => check.State == CheckState.Blocking);

        Assert.Equal("", blocking.Anchor.StepKey);
        Assert.Equal("Go to Settings · Discord", blocking.Anchor.Label);

        blocking.Anchor.Fix!.Execute(null);

        Assert.True(settings.IsOpen);
        Assert.Equal(SettingsSection.Discord, settings.OpenedAt);
    }

    /// <summary>The strip has no chip for a screen outside the flow, so the commit's own chip carries it.</summary>
    [Fact]
    public async Task TheLineStillBlocksTheCommit()
    {
        var (flow, _) = await WindowAsync(Unlinked());

        flow.CurrentStep = SetupSteps.SummaryKey;

        Assert.True(flow.Steps.Single(chip => chip.Key == SetupSteps.SummaryKey).IsBlocked);
    }

    /// <summary>A press on the strip's own settings button opens the dialog at its top.</summary>
    [Fact]
    public async Task OpeningTheDialogPlainlyStandsAtTheTop()
    {
        var (_, settings) = await WindowAsync(Unlinked());

        settings.OpenCommand.Execute(null);

        Assert.True(settings.IsOpen);
        Assert.Equal(SettingsSection.Top, settings.OpenedAt);
    }
}
