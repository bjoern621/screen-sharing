using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Fields.Model;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Shell.Settings.Model;
using ScreenShare.App.Features.Shell.Update.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.Settings.ViewModel;

/// <summary>
/// What the app does for itself: what it reports, what it reads on start, where its logs are,
/// which account it follows, which build it is, and whether the synthetic set runs beside it.
///
/// <b>Drawn over the window rather than in a destination.</b> None of it is about a stream, so no surface
/// configuring one owns it, and a reader who only watches reaches it all the same
/// (<see cref="GroupPlacement"/>).
///
/// <b>No commit.</b> The group is applied, so a control is the write and the next start reads it
/// (<c>docs/settings-editing.md</c>, "Staged and applied").
///
/// <b>The draft belongs to the window.</b> <see cref="FormSession"/> holds it, as it does for the wizard
/// and the watch panel, so this dialog's writes and a publish commit are writes of one message.
///
/// <b>Every read-back is read through.</b> The build, the link and the published release are the session's
/// and the update view model's, so the dialog and the status band cannot disagree.
///
/// <b>Outputs</b> only, written by <see cref="Apply"/> on every pass.
/// </summary>
public sealed class AppSettingsViewModel : Observable
{
    /// <summary>
    /// The keys this dialog places by name,
    /// under the heading each belongs to rather than in the group's own order (<see cref="Apply"/>).
    /// </summary>
    private const string CrashReportsKey = "app.send_crash_reports";
    private const string CheckUpdatesOnStartKey = "app.check_updates_on_start";
    private const string TestStreamsKey = "app.test_streams";

    private readonly FormSession _form;
    private readonly Session _session;
    private readonly UpdateViewModel _updates;

    /// <summary>One reset command per group key, kept so a pass produces an action equal to the last.</summary>
    private readonly Dictionary<string, DelegateCommand> _reset = [];

    /// <param name="backend">Reaches the log directory, which is the backend's own (<c>docs/ipc-api.md</c>).</param>
    /// <param name="form">Draft this window holds, and where a write leaves through.</param>
    /// <param name="session">Running state: the build this window talks to, and the Discord link.</param>
    /// <param name="updates">
    /// What the app knows about the published release, owned once for the window and drawn by the band as well.
    /// </param>
    /// <param name="dispatch">Hands an answer back to the UI loop.</param>
    public AppSettingsViewModel(
        IBackend backend, FormSession form, Session session, UpdateViewModel updates, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "app settings reach the backend for the files it owns");
        Assert.NotNull(form, "app settings draw the draft the window is holding");
        Assert.NotNull(session, "app settings read what the backend answered about this machine");
        Assert.NotNull(updates, "app settings draw the one answer about the published release");
        Assert.NotNull(dispatch, "app settings marshal an answer back to the UI loop");

        _form = form;
        _session = session;
        _updates = updates;

        Group = new FieldGroupViewModel(_form.Write, groupActionOf: ResetOf);

        // Named states rather than one toggle: a press asks for open or closed, and a second press
        // asks for what already holds (docs/development-principles.md, "Idempotency").
        OpenCommand = new DelegateCommand(() => Show(true));
        CloseCommand = new DelegateCommand(() => Show(false));

        // The logs are the backend's files: it writes them, rotates them, and is the side that knows
        // which still exist.
        OpenLogsFolder = new PendingCommand(() => backend.OpenLogsFolderAsync(), dispatch);

        Apply();
    }

    // --- Outputs ------------------------------------------------------------------

    private bool _isOpen;
    private string _version = "";
    private string _discordLine = "";
    private bool _isDiscordLinked;
    private bool _discordLineIsFailure;

    /// <summary>Whether the dialog stands over the window.</summary>
    public bool IsOpen { get => _isOpen; private set => Set(ref _isOpen, value); }

    public DelegateCommand OpenCommand { get; }

    public DelegateCommand CloseCommand { get; }

    public PendingCommand OpenLogsFolder { get; }

    /// <summary>The app group of the resolved form, holding the fields below and their write path.</summary>
    public FieldGroupViewModel Group { get; }

    private FieldViewModel? _crashReports;
    private FieldViewModel? _checkUpdatesOnStart;
    private FieldViewModel? _testStreams;

    /// <summary>Placed under the Logs heading, beside <see cref="OpenLogsFolder"/>.</summary>
    public FieldViewModel? CrashReports { get => _crashReports; private set => Set(ref _crashReports, value); }

    /// <summary>Placed under the Updates heading, beside <see cref="Updates"/>'s own check.</summary>
    public FieldViewModel? CheckUpdatesOnStart
    {
        get => _checkUpdatesOnStart;
        private set => Set(ref _checkUpdatesOnStart, value);
    }

    /// <summary>
    /// Placed under the Development heading, the synthetic set being a testing aid rather than something
    /// a stream is configured with.
    /// The backend converges the set on this write, so the streams come and go with the toggle
    /// (<c>backend/internal/app/teststreams.go</c>).
    /// </summary>
    public FieldViewModel? TestStreams { get => _testStreams; private set => Set(ref _testStreams, value); }

    /// <summary>Published release, the band's answer and this one being the same object.</summary>
    public UpdateViewModel Updates => _updates;

    /// <summary>Build the backend answered with, empty until the first read lands.</summary>
    public string Version { get => _version; private set => Set(ref _version, value); }

    /// <summary>Where this install stands with Discord, in one sentence.</summary>
    public string DiscordLine { get => _discordLine; private set => Set(ref _discordLine, value); }

    /// <summary>
    /// Whether that sentence reports something broken, which draws it in the failure hue
    /// (<c>docs/design-language.md</c>, "Palette").
    /// A link the manager declines is the one state here that is.
    /// </summary>
    public bool DiscordLineIsFailure
    {
        get => _discordLineIsFailure;
        private set => Set(ref _discordLineIsFailure, value);
    }

    public bool IsDiscordLinked { get => _isDiscordLinked; private set => Set(ref _isDiscordLinked, value); }

    /// <summary>Names the state the press asks for, so pressing twice changes nothing the second time.</summary>
    public void Show(bool open) => IsOpen = open;

    /// <summary>
    /// The one render function.
    /// Safe to run twice: an unmoved draft asks for no resolve, and the group's own pass produces fields
    /// that compare equal.
    /// </summary>
    public void Apply()
    {
        _form.Sync();

        var form = _form.Form;
        Group.Apply(GroupOf(form), _session.Words, form?.Settings, _form.IsAnswered);
        CrashReports = Group.Visible(CrashReportsKey);
        CheckUpdatesOnStart = Group.Visible(CheckUpdatesOnStartKey);
        TestStreams = Group.Visible(TestStreamsKey);

        Version = _session.Version;

        var discord = _session.Discord;
        IsDiscordLinked = discord?.Linked ?? false;
        DiscordLine = AppSettingsCopy.DiscordLine(discord);
        DiscordLineIsFailure = discord?.LinkRefused ?? false;

        Assert.That(
            Group.IsResolved || Group.Fields.Count == 0,
            "a dialog with no group draws no controls", Group.Fields.Count);
        Assert.That(
            !Group.IsResolved || CrashReports is not null,
            "a resolved app group always carries the crash-report toggle");
        Assert.That(
            !Group.IsResolved || CheckUpdatesOnStart is not null,
            "a resolved app group always carries the update-on-start toggle");
        Assert.That(
            !Group.IsResolved || TestStreams is not null,
            "a resolved app group always carries the test-stream toggle");
    }

    /// <summary>
    /// The reset beside the heading, which an applied group carries: its fields are already what this machine
    /// is, so a reader has no other way back to what a fresh installation holds
    /// (<c>docs/settings-editing.md</c>, "Who can be put back").
    ///
    /// Composed per pass around the held command, so two passes produce actions that compare equal.
    /// </summary>
    private GroupAction ResetOf(Api.V1.FieldGroup group) => new(
        AppSettingsCopy.ResetLabel, AppSettingsCopy.ResetHelp, ResetCommandOf(group.Key));

    private DelegateCommand ResetCommandOf(string key)
    {
        Assert.That(key.Length > 0, "a reset command is identified by the group it puts back");

        if (_reset.TryGetValue(key, out var command))
        {
            return command;
        }

        command = new DelegateCommand(() => _form.Reset(key));
        _reset[key] = command;
        return command;
    }

    /// <summary>
    /// App group of the resolved form, null where the form carries none.
    /// Found by the key deciding which surface draws a group, so this dialog and the wizard's filter read
    /// one table (<see cref="GroupPlacement"/>).
    /// </summary>
    private static Api.V1.FieldGroup? GroupOf(Api.V1.Form? form)
    {
        if (form is null)
        {
            return null;
        }

        foreach (var group in form.Groups)
        {
            if (GroupPlacement.InAppSettings(group.Key))
            {
                return group;
            }
        }

        return null;
    }
}
