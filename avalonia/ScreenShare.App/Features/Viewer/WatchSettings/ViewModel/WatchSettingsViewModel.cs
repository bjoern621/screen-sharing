using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.Model;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.WatchSettings.ViewModel;

/// <summary>
/// How this machine receives: the legs a stream comes back on, the windows a receiver holds packets in, and
/// the chain a tile converts decoded frames with.
///
/// <b>Drawn beside the tiles it governs, because that is where these settings act.</b> The wizard configures
/// what this machine <i>sends</i>, so a group placed there sends a reader who only watches into the broadcast
/// flow to change how their tiles decode.
/// It would also leave these settings unkept until a publish, the wizard's draft reaching the backend through
/// <c>StartPublish</c> (<see cref="GroupPlacement"/>).
///
/// <b>The draft belongs to the window.</b> <see cref="FormSession"/> holds it and the wizard reads the same
/// one, so this panel's writes and a publish commit are writes of one message.
/// A draft here would be the second copy of a fact that <c>docs/development-principles.md</c> forbids.
///
/// <b>What the commit reaches is the next decode.</b> A receive pipeline is built when it opens and takes no
/// value back afterwards, so a tile on screen keeps the chain it started with, the same fact that makes
/// <c>ApplyToStream</c> a method of its own on the publish side.
///
/// <b>Nothing here reaches a decode before the commit does.</b> The backend reads every knob a receive
/// pipeline needs out of its own settings as it builds the pipeline, and the one value the shell names in the
/// call, the tile's leg, is read from those same stored settings
/// (<c>Features/Viewer/Tile/Model/TileLeg.cs</c>).
/// Hence the panel saying when what it shows is not yet stored: a staged group draws the same controls kept or
/// not, leaving the button nothing to mean.
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
    /// Shuts the panel.
    /// Owned by the holding screen: whether a column is open belongs to that screen's arrangement, and this
    /// component draws one column of it (<c>Features/Viewer/ViewModel/ViewerViewModel.cs</c>).
    /// </summary>
    private readonly Action _close;

    /// <param name="dispatch">
    /// Hands work to the UI loop.
    /// A save answers on whichever thread the transport completed on, and a binding tolerates a write from the
    /// UI loop alone.
    /// </param>
    /// <param name="close">Shuts the panel, which a landed commit and the panel's own button both do.</param>
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

        // The renderer every wizard step goes through.
        // A group is the same group whichever screen places it, so this one needs no renderer of its own.
        Group = new FieldGroupViewModel(_form.Write);

        // An effect: a round trip the backend can refuse.
        // The command holds whether one is out, which is what the button waits on and what refuses a second
        // press.
        SaveCommand = new PendingCommand(SaveAsync, dispatch, () => _form.Draft is not null);
        SaveCommand.Changed += Apply;

        // Plain: dismissal costs no round trip and cannot be refused.
        // It keeps nothing, a reader who moved a control and shut the panel having decided against it, and
        // storing on the way out would keep what nobody asked to have kept.
        CloseCommand = new DelegateCommand(_close);

        // Nothing subscribed to here.
        // The screen holding this panel renders it from its own pass, the arrangement every child component on
        // this shell has: one notification, one render, no change rendered twice.
        Apply();
    }

    // --- Outputs ------------------------------------------------------------------

    private string _notice = "";
    private bool _hasNotice;
    private bool _isUnkept;

    /// <summary>The controls, through the renderer the wizard's steps share.</summary>
    public FieldGroupViewModel Group { get; }

    /// <summary>Keeps these settings, for the decodes this machine opens next.</summary>
    public PendingCommand SaveCommand { get; }

    /// <summary>Shuts the panel, keeping nothing.</summary>
    public DelegateCommand CloseCommand { get; }

    /// <summary>The half of what the button does that a label has no room for.</summary>
    public string SaveTip =>
        "Keeps these settings. A tile already on screen keeps the pipeline it was opened with, so a change reaches the next decode rather than the running one.";

    /// <summary>What the close control does, a glyph being no sentence.</summary>
    public string CloseTip => "Closes this panel. Anything not kept is left as it was.";

    /// <summary>The backend's sentence about the last save, empty where there is none.</summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    /// <summary>
    /// Whether the panel shows something other than what the backend holds.
    ///
    /// The only way to see the difference a staged group makes: the controls draw the draft either way.
    /// Raised by a field the reader moved and has not kept, and by a value the resolve repaired that nothing
    /// wrote back, which mean the same for a decode: it opens on the held value and not on the one on screen.
    /// </summary>
    public bool IsUnkept { get => _isUnkept; private set => Set(ref _isUnkept, value); }

    /// <summary>What being unkept costs, where the reader is looking while it is true.</summary>
    public string UnkeptNotice =>
        "Not kept yet. A decode opens on the settings that were kept last, so these reach one when the button below is pressed.";

    /// <summary>
    /// The one render function.
    /// Safe to run twice: a draft that has not moved asks for no resolve, and the group's own pass produces
    /// fields that compare equal, so a repeated pass fires no binding.
    /// </summary>
    public void Apply()
    {
        // Reconciled from the pass rather than performed by it, as the wizard does it: the pass names the
        // state it wants, a resolved form, and the converge decides whether anything is asked
        // (docs/development-principles.md, "Idempotency").
        _form.Sync();

        var form = _form.Form;
        var group = GroupOf(form);
        Group.Apply(group, _session.Words, form?.Settings);

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
    ///
    /// Compared over the form's own keys rather than a list written here, so a knob the backend adds to the
    /// group is compared with nothing here to edit.
    ///
    /// A side that has not arrived is no difference.
    /// Before the first answer nothing is on screen to be unkept, and saying otherwise would put a warning
    /// under a panel holding no controls.
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
    /// The watch group of the resolved form, null where the form carries none.
    /// Found by the key that decides which screen draws a group, so this panel and the wizard's own filter
    /// read one table (<see cref="GroupPlacement"/>).
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
    /// It reaches no backend itself, which is the point.
    /// Settings travel whole, so this commit and an applied field's keystroke are writes of one message.
    /// Sent from two places they are unary calls with no ordering between them, and the older snapshot landing
    /// last is a stored setting the reader had already moved off.
    /// One queue removes that (<see cref="FormSession.SaveAsync"/>).
    ///
    /// <b>Safe to press twice.</b> The call names a state, that these are the held settings, so a second press
    /// over an unchanged draft asks for a state that holds, which is a success
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
    ///
    /// <b>A write that landed shuts the panel, one that did not leaves it open.</b> Once the settings are
    /// kept, this column holds nothing left to look at, and a panel that stayed would be closed a second time
    /// by hand.
    /// A refusal is the opposite: the sentence explaining it sits on this panel, so dismissing takes it off
    /// screen along with the fields it is about.
    /// </summary>
    private void Kept()
    {
        // Read through rather than carried back through the commit, the write reporting its own answer.
        // A copy would be one sentence held twice.
        if (_form.Unsaved.Length == 0)
        {
            _close();
            return;
        }

        Apply();
    }
}
