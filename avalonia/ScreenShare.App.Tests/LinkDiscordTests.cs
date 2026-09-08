using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.ViewModel;
using ScreenShare.App.Features.Shell.Settings.ViewModel;
using ScreenShare.App.Features.Shell.Update.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The settings dialog carries the button that links this install.
/// A link is one per computer and configures no stream, so it stands with the settings about the app
/// rather than on the wizard step where a stream picks its group.
/// The secret lands in the stored settings on the backend's side,
/// which is why the press writes nothing here and the line reads the session's Discord state.
/// </summary>
public sealed class LinkDiscordTests
{
    private static readonly Action<Action> Inline = action => action();

    /// <summary>Waits for a press that crossed the backend, the call answering off this thread.</summary>
    private static async Task Eventually(Func<bool> landed)
    {
        for (var i = 0; i < 200 && !landed(); i++)
        {
            await Task.Delay(10);
        }

        Assert.True(landed());
    }

    /// <summary>The dialog on its own draft, built the way the window builds it.</summary>
    private static async Task<AppSettingsViewModel> DialogAsync(SeededBackend backend)
    {
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        var updates = new UpdateViewModel(backend, session, Inline);
        var settings = new AppSettingsViewModel(backend, form, session, updates, Inline);

        // Stands in for the shell's own render pass (Features/Shell/ViewModel/ShellViewModel.cs).
        session.Changed += settings.Apply;

        _ = session.Start();
        session.Stop();

        await form.Settled;
        return settings;
    }

    [Fact]
    public async Task TheDialogCarriesTheButtonThatLinks()
    {
        var settings = await DialogAsync(new SeededBackend("linux"));

        Assert.Equal("Link Discord", settings.LinkLabel);
        Assert.True(settings.LinkDiscord.CanExecute(null));
    }

    [Fact]
    public async Task APressRunsTheLinkAgainstTheDraftsRelay()
    {
        var backend = new SeededBackend("linux");
        var settings = await DialogAsync(backend);

        settings.LinkDiscord.Execute(null);

        await Eventually(() => backend.DiscordLinks.Count == 1);
        Assert.Equal(backend.RelayHost, backend.DiscordLinks[0].Host);
    }

    /// <summary>
    /// A linked install can press again, the press putting another account where the linked one stands,
    /// so the label says which of the two presses this is rather than offering a link already made.
    /// </summary>
    [Fact]
    public async Task ALinkedInstallIsOfferedAnotherAccount()
    {
        var backend = new SeededBackend("linux") { Discord = new DiscordState { Linked = true } };

        var settings = await DialogAsync(backend);

        Assert.Equal("Link a different account", settings.LinkLabel);
    }

    /// <summary>
    /// A refused link is held by this install and declined by the manager, so the press it offers
    /// draws a fresh secret for the account already named.
    /// </summary>
    [Fact]
    public async Task ARefusedLinkIsOfferedALinkAgain()
    {
        var backend = new SeededBackend("linux")
        {
            Discord = new DiscordState { Linked = true, AccountName = "bjoern", LinkRefused = true },
        };

        var settings = await DialogAsync(backend);

        Assert.Equal("Link Discord again", settings.LinkLabel);
    }

    /// <summary>The manager sits beside the relay, so a machine pointed at none has nothing to link against.</summary>
    [Fact]
    public async Task AMachineWithNoRelayCannotLink()
    {
        var settings = await DialogAsync(new SeededBackend("linux") { RelayHost = "" });

        Assert.False(settings.LinkDiscord.CanExecute(null));
        Assert.NotEmpty(settings.LinkHint);
    }

    /// <summary>
    /// The relay step decides which group a stream goes to, and the link is neither that nor per stream,
    /// so the choice naming the voice channel carries no button of its own.
    /// </summary>
    [Fact]
    public async Task TheRelayStepCarriesNoLinkButton()
    {
        var flow = Flows.Setup(new SeededBackend("linux"));
        await flow.Settled;
        flow.CurrentStep = "relay";

        var choice = flow.CurrentGroup!.Fields.Single(field => field.Key == "relay.group_source");

        Assert.False(choice.HasAction);
    }
}
