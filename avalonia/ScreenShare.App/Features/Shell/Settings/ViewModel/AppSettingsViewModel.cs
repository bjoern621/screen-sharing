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
/// which account it follows and which build it is.
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

    /// <summary>Whether the dialog stands over the window.</summary>
    public bool IsOpen { get => _isOpen; private set => Set(ref _isOpen, value); }

    public DelegateCommand OpenCommand { get; }

    public DelegateCommand CloseCommand { get; }

    public PendingCommand OpenLogsFolder { get; }

    /// <summary>The app group of the resolved form, drawn by the renderer every other screen shares.</summary>
    public FieldGroupViewModel Group { get; }

    /// <summary>Published release, the band's answer and this one being the same object.</summary>
    public UpdateViewModel Updates => _updates;

    /// <summary>Build the backend answered with, empty until the first read lands.</summary>
    public string Version { get => _version; private set => Set(ref _version, value); }

    /// <summary>Where this install stands with Discord, in one sentence.</summary>
    public string DiscordLine { get => _discordLine; private set => Set(ref _discordLine, value); }

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

        Version = _session.Version;

        var discord = _session.Discord;
        IsDiscordLinked = discord?.Linked ?? false;
        DiscordLine = AppSettingsCopy.DiscordLine(discord);

        Assert.That(
            Group.IsResolved || Group.Fields.Count == 0,
            "a dialog with no group draws no controls", Group.Fields.Count);
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
