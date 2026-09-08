using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Shell.Consent.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The one question an install answers before anything else: whether a crash log leaves this computer.
///
/// Drawn over the window while the stored settings say it stands unput, and closed by the answer
/// (<c>backend/internal/settings/app.go</c>, <c>CrashReportsAsked</c>).
/// The toggle in it is the app group's own field, so the answer is stored the way every other applied
/// setting is.
/// </summary>
public sealed class ConsentTests
{
    private static readonly Action<Action> Inline = action => action();

    private const string CrashKey = "app.send_crash_reports";

    private sealed record Panel(ConsentViewModel Consent, FormSession Form, SeededBackend Backend);

    /// <summary>The dialog on its own draft, built the way the window builds it.</summary>
    private static async Task<Panel> PanelAsync(bool asked = false)
    {
        var backend = new SeededBackend("linux");
        var stored = await backend.SettingsAsync();
        stored.App.CrashReportsAsked = asked;
        backend.Stored = stored;

        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        var consent = new ConsentViewModel(form, session);

        // Stands in for the shell's own render pass (Features/Shell/ViewModel/ShellViewModel.cs).
        session.Changed += consent.Apply;

        _ = session.Start();
        session.Stop();

        await form.Settled;
        return new Panel(consent, form, backend);
    }

    /// <summary>An install nobody has asked draws the question, with the toggle the answer is given on.</summary>
    [Fact]
    public async Task AnUnaskedInstallDrawsTheQuestion()
    {
        var panel = await PanelAsync();

        Assert.True(panel.Consent.IsOpen);
        Assert.NotNull(panel.Consent.CrashReports);
        Assert.Equal(CrashKey, panel.Consent.CrashReports!.Key);
    }

    /// <summary>An install carrying an answer draws nothing, the question being one a reader meets once.</summary>
    [Fact]
    public async Task AnAnsweredInstallDrawsNothing()
    {
        var panel = await PanelAsync(asked: true);

        Assert.False(panel.Consent.IsOpen);
    }

    /// <summary>
    /// Closing is the answer, so it stores that the question was put.
    /// The dialog is drawn off that stored fact, so the same write is what closes it.
    /// </summary>
    [Fact]
    public async Task ClosingStoresThatTheQuestionWasPut()
    {
        var panel = await PanelAsync();

        panel.Consent.CloseCommand.Execute(null);
        await panel.Form.Settled;

        var saved = Assert.Single(panel.Backend.Saved);
        Assert.True(saved.App.CrashReportsAsked);
        Assert.False(panel.Consent.IsOpen);
    }

    /// <summary>
    /// Closing names the state it wants, so a second press asks for what already holds
    /// (<c>docs/development-principles.md</c>, "Idempotency").
    /// </summary>
    [Fact]
    public async Task ClosingTwiceStoresOneAnswer()
    {
        var panel = await PanelAsync();

        panel.Consent.CloseCommand.Execute(null);
        panel.Consent.CloseCommand.Execute(null);
        await panel.Form.Settled;

        Assert.Single(panel.Backend.Saved);
    }

    /// <summary>
    /// The toggle is the app group's field, and that group is applied, so moving it is the write
    /// (<c>docs/settings-editing.md</c>, "Staged and applied").
    /// </summary>
    [Fact]
    public async Task TheToggleIsTheWrite()
    {
        var panel = await PanelAsync();

        panel.Consent.CrashReports!.Flag = false;
        await panel.Form.Settled;

        Assert.Contains(panel.Backend.Saved, saved => !saved.App.SendCrashReports);
    }
}
