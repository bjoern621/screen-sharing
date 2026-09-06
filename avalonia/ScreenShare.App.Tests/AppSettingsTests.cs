using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Fields.Model;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Shell.Settings.ViewModel;
using ScreenShare.App.Features.Shell.Update.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The settings about the app rather than about a stream: what it reports, what it reads on start,
/// and where its logs are.
///
/// Drawn over the window instead of in the wizard, and kept as they are moved, the group being applied
/// (<c>docs/settings-editing.md</c>).
/// </summary>
public sealed class AppSettingsTests
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

    private const string CrashKey = "app.send_crash_reports";

    private sealed record Panel(AppSettingsViewModel Settings, FormSession Form, SeededBackend Backend);

    /// <summary>The dialog on its own draft, built the way the window builds it.</summary>
    private static async Task<Panel> PanelAsync(SeededBackend? seed = null)
    {
        var backend = seed ?? new SeededBackend("linux");
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        var updates = new UpdateViewModel(backend, session, Inline);
        var settings = new AppSettingsViewModel(backend, form, session, updates, Inline);

        // Stands in for the shell's own render pass (Features/Shell/ViewModel/ShellViewModel.cs).
        session.Changed += settings.Apply;

        _ = session.Start();
        session.Stop();

        await form.Settled;
        return new Panel(settings, form, backend);
    }

    /// <summary>The group the backend answers with, drawn by the renderer every other screen shares.</summary>
    [Fact]
    public async Task TheDialogDrawsTheAppGroup()
    {
        var panel = await PanelAsync();
        var group = (await panel.Backend.ResolveFormAsync(await panel.Backend.SettingsAsync()))
            .Groups.Single(g => GroupPlacement.InAppSettings(g.Key));

        Assert.True(panel.Settings.Group.IsResolved);
        Assert.Equal(
            group.Fields.Where(field => field.Visible).Select(field => field.Key),
            panel.Settings.Group.Fields.Select(field => field.Key));
    }

    /// <summary>
    /// The wizard configures what this machine sends, so a group about the app is no step of it.
    /// A reader who never publishes still reaches these.
    /// </summary>
    [Fact]
    public async Task TheWizardDrawsNoStepForTheAppGroup()
    {
        var panel = await PanelAsync();
        var form = await panel.Backend.ResolveFormAsync(await panel.Backend.SettingsAsync());

        var steps = SetupSteps.For(form.Groups.Where(g => GroupPlacement.InSetup(g.Key)).ToList());

        Assert.DoesNotContain(steps, step => GroupPlacement.InAppSettings(step.Key));
        Assert.Contains(form.Groups, g => GroupPlacement.InAppSettings(g.Key));
    }

    /// <summary>
    /// The group is applied, so the toggle is the write.
    /// Nothing here waits for a publish: the next start is what reads the value, and a dialog with a commit
    /// would leave a reader who closed it wondering which answer stands.
    /// </summary>
    [Fact]
    public async Task AToggleIsStoredWithNoCommitBesideIt()
    {
        var panel = await PanelAsync();

        Field(panel, CrashKey).Flag = false;
        await panel.Form.Settled;

        var saved = Assert.Single(panel.Backend.Saved);
        Assert.False(saved.App.SendCrashReports);
        Assert.Empty(panel.Backend.Started);
    }

    /// <summary>
    /// Opening and closing name the state they want rather than toggling one,
    /// so a second press asks for what already holds (<c>docs/development-principles.md</c>).
    /// </summary>
    [Fact]
    public async Task OpeningAndClosingNameTheState()
    {
        var panel = await PanelAsync();
        Assert.False(panel.Settings.IsOpen);

        panel.Settings.OpenCommand.Execute(null);
        panel.Settings.OpenCommand.Execute(null);
        Assert.True(panel.Settings.IsOpen);

        panel.Settings.CloseCommand.Execute(null);
        panel.Settings.CloseCommand.Execute(null);
        Assert.False(panel.Settings.IsOpen);
    }

    /// <summary>
    /// The link state is read through from the session rather than kept here,
    /// so the dialog and the wizard report one answer.
    /// </summary>
    [Fact]
    public async Task TheDiscordLineNamesTheChannelBeingFollowed()
    {
        var backend = new SeededBackend("linux")
        {
            Discord = new DiscordState
            {
                Linked = true,
                InChannel = true,
                GuildName = "Fixture guild",
                ChannelName = "General",
            },
        };

        var panel = await PanelAsync(backend);

        Assert.Contains("General", panel.Settings.DiscordLine);
        Assert.Contains("Fixture guild", panel.Settings.DiscordLine);
    }

    /// <summary>An install with no linked account says so, rather than drawing an empty channel.</summary>
    [Fact]
    public async Task TheDiscordLineSaysWhenNothingIsLinked()
    {
        var panel = await PanelAsync();

        Assert.False(panel.Settings.IsDiscordLinked);
        Assert.NotEmpty(panel.Settings.DiscordLine);
    }

    /// <summary>The build this window is running, read off the session that asked for it.</summary>
    [Fact]
    public async Task TheVersionIsTheOneTheBackendAnswered()
    {
        var panel = await PanelAsync();

        Assert.Equal(SeededBackend.Build, panel.Settings.Version);
    }

    /// <summary>The logs are the backend's files, so the press is a backend call.</summary>
    [Fact]
    public async Task OpeningTheLogsFolderAsksTheBackend()
    {
        var panel = await PanelAsync();

        panel.Settings.OpenLogsFolder.Execute(null);
        await Eventually(() => panel.Backend.LogsFolderOpened == 1);
    }

    private static Features.Fields.ViewModel.FieldViewModel Field(Panel panel, string key)
        => panel.Settings.Group.Fields.Single(field => field.Key == key);
}
