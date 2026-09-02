using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.Model;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.WatchSettings.ViewModel;

/// <summary>
/// How this machine receives: the legs a stream comes back on, the windows a receiver holds packets in,
/// and the chain a tile converts decoded frames with.
///
/// <b>Drawn beside the tiles it governs.</b> The wizard configures what this machine <i>sends</i>, so a group
/// placed there sends a reader who only watches into the broadcast flow,
/// and leaves these settings unkept until a publish, the wizard's draft reaching the backend
/// through <c>StartPublish</c> (<see cref="GroupPlacement"/>).
///
/// <b>The draft belongs to the window.</b> <see cref="FormSession"/> holds it and the wizard reads the same one,
/// so this panel's writes and a publish commit are writes of one message.
/// A draft here would be the second copy of a fact <c>docs/development-principles.md</c> forbids.
///
/// <b>A commit reaches the next decode.</b> A receive pipeline is built when it opens and takes no value back
/// afterwards, so a tile on screen keeps the chain it started with.
///
/// <b>Nothing here reaches a decode before the commit does.</b> The backend reads every knob a receive pipeline
/// needs out of its own settings as it builds the pipeline,
/// and the leg the shell names in the call is read from those same stored settings
/// (<c>Features/Viewer/Tile/Model/TileLeg.cs</c>).
/// Hence the panel saying when what it shows differs from what is stored: a staged group draws the same controls
/// either way, leaving the button nothing to mean.
///
/// Which controls the group holds, which of their entries are reachable and why an unreachable one is greyed
/// arrive decided on the form (<c>docs/field-availability.md</c>).
///
/// <b>Outputs</b> only, written by <see cref="Apply"/> on every pass.
/// A write leaves through the field the reader moved, into the one draft.
/// </summary>
public sealed class WatchSettingsViewModel : Observable
{
    private readonly FormSession _form;
    private readonly Session _session;
    private readonly Action<Action> _dispatch;

    /// <summary>
    /// Owned by the holding screen: whether a column is open belongs to that screen's arrangement,
    /// and this component draws one column of it (<c>Features/Viewer/ViewModel/ViewerViewModel.cs</c>).
    /// </summary>
    private readonly Action _close;

    /// <summary>One reset command per group key, kept so a pass produces an action equal to the last.</summary>
    private readonly Dictionary<string, DelegateCommand> _reset = [];

    /// <param name="form">
    /// Draft this window holds, and where a write leaves through.
    /// The panel keeps no copy: the group renders what the session answers on each pass.
    /// </param>
    /// <param name="session">
    /// Names the entries the group offers, the vocabulary being the backend's and the words this side's.
    /// </param>
    /// <param name="dispatch">
    /// Hands work to the UI loop.
    /// A save answers on whichever thread the transport completed on,
    /// and a binding tolerates a write from the UI loop alone.
    /// </param>
    /// <param name="close">Shuts the panel, called by a landed commit and by the panel's own button.</param>
    public WatchSettingsViewModel(
        FormSession form, Session session, Action<Action> dispatch, Action close)
    {
        Assert.NotNull(form, "a watch panel draws the draft the window is holding");
        Assert.NotNull(session, "a watch panel needs the vocabulary that names its entries");
        Assert.NotNull(dispatch, "a watch panel needs a UI loop to marshal an answer back to");
        Assert.NotNull(close, "a watch panel needs a way to say it is done");

        _form = form;
        _session = session;
        _dispatch = dispatch;
        _close = close;

        // Renderer every wizard step goes through: a group is the same group whichever screen places it.
        Group = new FieldGroupViewModel(_form.Write, groupActionOf: ResetOf, sweep: _form.Sweeping);

        // Round trip the backend can refuse.
        // The command holds whether one is out, which the button waits on and which refuses a second press.
        SaveCommand = new PendingCommand(SaveAsync, dispatch, () => _form.Draft is not null);
        SaveCommand.Changed += Apply;

        // Plain: dismissal costs no round trip and cannot be refused.
        // Keeps nothing, shutting the panel over a moved control being deciding against it.
        CloseCommand = new DelegateCommand(_close);

        // Nothing subscribed to here.
        // The holding screen renders this panel from its own pass, the arrangement every child on this shell has:
        // one notification, one render.
        Apply();
    }

    // --- Outputs ------------------------------------------------------------------

    private string _notice = "";
    private bool _hasNotice;
    private bool _isUnkept;

    public FieldGroupViewModel Group { get; }

    public PendingCommand SaveCommand { get; }

    public DelegateCommand CloseCommand { get; }

    public string SaveTip =>
        "Keeps these settings. A tile already on screen keeps what it opened with, so a change reaches the next tile you open.";

    public string CloseTip => "Closes this panel. Anything not kept is left as it was.";

    /// <summary>
    /// The reset offered beside the heading, and the one place a staged group carries one.
    /// The wizard's rule is that an applied group has no other way back, a staged one being a proposal a reader
    /// walks away from (<c>Features/Setup/ViewModel/SetupViewModel.cs</c>).
    /// This group is staged and keeps itself, so after a press on the commit walking away restores nothing
    /// (<c>docs/settings-editing.md</c>, "Staged and applied").
    ///
    /// It writes the draft and stores nothing, which is what every control on this panel does.
    /// The values are the form's, stated per field, so no default is named here.
    ///
    /// Composed per pass around the held command, so two passes produce actions that compare equal
    /// (<see cref="GroupAction"/>).
    /// </summary>
    private GroupAction ResetOf(Api.V1.FieldGroup group) => new(
        "Reset to defaults",
        "Puts every setting here back to the value a fresh installation starts with. Press Keep these settings to store them.",
        ResetCommandOf(group.Key));

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

    /// <summary>Backend's sentence about the last save, empty where there is none.</summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    /// <summary>
    /// Whether the panel shows something other than what the backend holds.
    /// The only way to see it: the controls draw the draft either way.
    /// Raised by a field the reader moved and by a value the resolve repaired that nothing wrote back,
    /// which mean the same for a decode: it opens on the held value and not on the one on screen.
    /// </summary>
    public bool IsUnkept { get => _isUnkept; private set => Set(ref _isUnkept, value); }

    public string UnkeptNotice =>
        "These differ from the kept settings. A new tile opens on the kept ones, so press Keep these settings for these to take effect.";

    /// <summary>
    /// The one render function.
    /// Safe to run twice: an unmoved draft asks for no resolve,
    /// and the group's own pass produces fields that compare equal, so a repeated pass fires no binding.
    /// </summary>
    public void Apply()
    {
        // Reconciled from the pass rather than performed by it: the pass names the state it wants, a resolved
        // form, and the converge decides whether anything is asked (docs/development-principles.md,
        // "Idempotency").
        _form.Sync();

        var form = _form.Form;
        var group = GroupOf(form);
        Group.Apply(group, _session.Words, form?.Settings, _form.IsAnswered);

        // Read through rather than held: this panel and the wizard write one settings message down one queue,
        // so what could not be stored is a single sentence and never one per screen (FormSession.Unsaved).
        Notice = _form.Unsaved;
        HasNotice = Notice.Length > 0;
        IsUnkept = Unkept(group, _form.Draft, _form.Stored);
        SaveCommand.Refresh();

        Assert.That(HasNotice == (Notice.Length > 0), "the notice and its sentence agree", Notice);
        Assert.That(
            Group.IsResolved || Group.Fields.Count == 0,
            "a panel with no group draws no controls", Group.Fields.Count);
    }

    /// <summary>
    /// Whether the group's fields hold something other than what the backend holds.
    /// Compared over the form's own keys and not a list written here, so a knob the backend adds needs no edit.
    /// A side that has not arrived is no difference: a warning under a panel holding no controls says nothing.
    /// </summary>
    private static bool Unkept(Api.V1.FieldGroup? group, Api.V1.Settings? draft, Api.V1.Settings? stored)
    {
        if (group is null || draft is null || stored is null)
        {
            return false;
        }

        foreach (var field in group.Fields)
        {
            if (!SettingsDraft.Read(draft, field.Key).Equals(SettingsDraft.Read(stored, field.Key)))
            {
                return true;
            }
        }

        return false;
    }

    /// <summary>
    /// Watch group of the resolved form, null where the form carries none.
    /// Found by the key deciding which screen draws a group, so this panel and the wizard's filter read one table
    /// (<see cref="GroupPlacement"/>).
    /// </summary>
    private static Api.V1.FieldGroup? GroupOf(Api.V1.Form? form)
    {
        if (form is null)
        {
            return null;
        }

        foreach (var group in form.Groups)
        {
            if (GroupPlacement.InViewer(group.Key))
            {
                return group;
            }
        }

        return null;
    }

    /// <summary>
    /// Keeps the draft, through the writer the window shares.
    ///
    /// Reaches no backend itself.
    /// Settings travel whole, so this commit and an applied field's keystroke are writes of one message.
    /// Sent from two places they are unary calls with no ordering between them, and the older snapshot landing
    /// last is a stored setting the reader had already moved off.
    /// One queue removes that (<see cref="FormSession.SaveAsync"/>).
    ///
    /// <b>Safe to press twice.</b> The call names a state, that these are the held settings,
    /// so a second press over an unchanged draft asks for a state that holds, a success
    /// (<c>docs/development-principles.md</c>, "Effects across a process boundary").
    /// </summary>
    private async Task SaveAsync()
    {
        await _form.SaveAsync().ConfigureAwait(false);

        // Queued behind the record of the write's answer rather than racing it: both cross this dispatcher,
        // and the write handed its own over before the awaited task completed.
        _dispatch(Kept);
    }

    /// <summary>
    /// Ends the commit, on the UI loop.
    /// <b>A write that landed shuts the panel, one that did not leaves it open.</b>
    /// Kept settings leave this column nothing to look at, and a panel that stayed would be closed by hand.
    /// A refusal's sentence sits on this panel, so dismissing takes it off screen with the fields it is about.
    /// </summary>
    private void Kept()
    {
        // Read through rather than carried back through the commit: a copy would be one sentence held twice.
        if (_form.Unsaved.Length == 0)
        {
            _close();
            return;
        }

        Apply();
    }
}
