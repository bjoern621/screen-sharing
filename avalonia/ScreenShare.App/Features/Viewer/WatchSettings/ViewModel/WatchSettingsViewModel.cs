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
/// <b>It is here because this is where those settings do something.</b> They were a step of the setup wizard,
/// which is the screen for configuring what this machine <i>sends</i> - so a reader setting up a broadcast
/// walked past a page of receiving settings, and a reader who was only watching had to open the broadcast
/// wizard to change how their tiles decode.
/// Worse, the only thing that persisted them was starting to share: the wizard's draft reaches the backend
/// through <c>StartPublish</c>, so a reader who never published never saved a render chain.
/// Both are the same misplacement, and moving the group fixes both (<see cref="GroupPlacement"/>).
///
/// <b>The draft is not this class's.</b> It reads <see cref="FormSession"/>, which the window owns and the
/// setup wizard reads too, so the settings this panel writes and the settings a publish commits are one
/// message.
/// A draft of its own would be the second copy that <c>docs/development-principles.md</c> exists to forbid.
///
/// <b>Saving is explicit, and what it reaches is the next decode.</b> Both receive pipelines are built when
/// they are opened and neither takes a value back afterwards, so a tile already on screen keeps the chain it
/// started with - the same fact that makes <c>ApplyToStream</c> a separate method on the publish side.
/// The button says so rather than the panel pretending a slider reaches a running pipeline.
///
/// <b>Nothing here reaches a decode before the button does.</b> Every knob a receive pipeline reads is read
/// by the backend out of its own settings as the pipeline is built, and the one value the shell names in the
/// call - the tile's leg - is read out of those same stored settings
/// (<c>Features/Viewer/Tile/Model/TileLeg.cs</c>).
/// Which is why the panel says when what it shows is not yet what is stored: a staged group that looked
/// identical whether or not it had been kept would give the button nothing to mean.
///
/// <b>Outputs</b> only, written by <see cref="Apply"/> on every pass.
/// Every write leaves through the field the reader moved, into the one draft.
/// </summary>
public sealed class WatchSettingsViewModel : Observable
{
    private readonly FormSession _form;
    private readonly Session _session;
    private readonly Action<Action> _dispatch;

    /// <summary>
    /// Shuts the panel.
    /// It is the holding screen's, because whether a panel is open is part of that screen's arrangement and
    /// this component draws one column of it (<c>Features/Viewer/ViewModel/ViewerViewModel.cs</c>).
    /// </summary>
    private readonly Action _close;

    /// <param name="dispatch">
    /// Hands work to the UI loop.
    /// The answer to a save lands on whichever thread the transport completed on, and everything this writes
    /// is read by a binding that only tolerates being written from one.
    /// </param>
    /// <param name="close">Shuts the panel, which a commit and the panel's own button both do.</param>
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

        // The same generic renderer every step of the wizard uses.
        // What the group contains and which of its entries are reachable is the form's answer, unchanged by
        // which screen draws it - which is the whole reason moving the group costs no renderer.
        Group = new FieldGroupViewModel(_form.Write);

        // Persisting is an effect: it takes a round trip and the backend can refuse it.
        // The command holds whether one is in flight, which is both what the button waits on and what refuses
        // a second press.
        SaveCommand = new PendingCommand(SaveAsync, dispatch, () => _form.Draft is not null);
        SaveCommand.Changed += Apply;

        // Dismissing takes no round trip and cannot be refused, so it is the plain command.
        // It keeps nothing: a reader who moved a slider and shut the panel decided against it, and storing on
        // the way out would be this screen keeping what nobody asked it to.
        CloseCommand = new DelegateCommand(_close);

        // Nothing is subscribed to here.
        // The screen that holds this panel renders it from its own pass, which is the arrangement every other
        // child component on this shell has: one notification, one render, and no two components rendering
        // the same change twice.
        Apply();
    }

    // --- Outputs ------------------------------------------------------------------

    private string _notice = "";
    private bool _hasNotice;
    private bool _isUnkept;

    /// <summary>The controls, drawn by the one generic renderer the wizard's steps use.</summary>
    public FieldGroupViewModel Group { get; }

    /// <summary>Keeps these settings for the decodes this machine opens next.</summary>
    public PendingCommand SaveCommand { get; }

    /// <summary>Shuts the panel, keeping nothing.</summary>
    public DelegateCommand CloseCommand { get; }

    /// <summary>What the button says it does, which is the half of it a label cannot carry.</summary>
    public string SaveTip =>
        "Keeps these settings. A tile already on screen keeps the pipeline it was opened with, so a change reaches the next decode rather than the running one.";

    /// <summary>What the close button says it does, since a glyph is not a sentence.</summary>
    public string CloseTip => "Closes this panel. Anything not kept is left as it was.";

    /// <summary>What the backend said about the last save, empty otherwise.</summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    /// <summary>
    /// Whether what the panel shows is not what the backend is holding.
    ///
    /// It is the whole difference a staged group makes, and there is no other way to see it: the controls
    /// draw the draft either way.
    /// Two things raise it - a field the reader moved and has not kept, and a value the resolve repaired that
    /// nothing has written back - and both mean the same thing for a decode, which is that it will open on
    /// the held value and not on the one on screen.
    /// </summary>
    public bool IsUnkept { get => _isUnkept; private set => Set(ref _isUnkept, value); }

    /// <summary>What being unkept costs, in the place the reader is looking when it is true.</summary>
    public string UnkeptNotice =>
        "Not kept yet. A decode opens on the settings that were kept last, so these reach one when the button below is pressed.";

    /// <summary>
    /// The one render function.
    /// Safe to run twice: the converge it asks for is skipped when the draft has not moved, and the group's
    /// own pass produces fields that compare equal, so an unchanged pass fires no binding.
    /// </summary>
    public void Apply()
    {
        // Reconciled from the render pass rather than performed by it, exactly as the wizard does it: the
        // pass states that it wants a form, and the converge decides whether anything has to be asked
        // (docs/development-principles.md, "Idempotency").
        _form.Sync();

        var form = _form.Form;
        var group = GroupOf(form);
        Group.Apply(group, _session.Words, form?.Settings);

        // The failure the whole window shares, read through rather than kept: this panel and the wizard write
        // the same settings message down the same queue, so what could not be stored is one sentence and not
        // one per screen (FormSession.Unsaved).
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
    /// Whether the group's fields hold something other than what the backend is holding.
    ///
    /// The keys are the form's own rather than a list written here, so a knob the backend adds to the group
    /// is compared with nothing to edit - the same property that makes the panel draw it in the first place.
    ///
    /// Neither side arriving is not a difference.
    /// Before the first answer there is nothing on screen to be unkept, and saying so would put a warning
    /// under a panel with no controls in it.
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
    /// The watch group of the resolved form, or null where the form carries none.
    /// Looked up by the key that says which screen draws it, so this panel and the wizard's own filter read
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
            if (GroupPlacement.InViewer(group.Key))
            {
                return group;
            }
        }

        return null;
    }

    /// <summary>
    /// Keeps the draft, through the writer the whole window shares.
    ///
    /// It does not reach the backend itself, and that is the point.
    /// The settings travel whole, so this commit and an applied field's keystroke are two writes of one
    /// message; sent from two places they are two unary calls with no ordering between them, and the older
    /// snapshot landing last is a stored setting the reader had already changed.
    /// One queue removes that (<see cref="FormSession.SaveAsync"/>).
    ///
    /// <b>Safe to press twice.</b> The call names a state - these are the held settings - so a second press
    /// with nothing changed asks for a state that already holds, which is a success
    /// (<c>docs/development-principles.md</c>, "Effects across a process boundary").
    /// </summary>
    private async Task SaveAsync()
    {
        await _form.SaveAsync().ConfigureAwait(false);

        // Back to the UI loop, where the answer to the write was recorded.
        // It is queued behind that record rather than racing it: both go through this dispatcher, and the
        // write's own was handed over before the task this awaited completed.
        _dispatch(Kept);
    }

    /// <summary>
    /// Ends the commit, on the UI loop.
    ///
    /// <b>A write that landed shuts the panel and one that did not leaves it open.</b> The reader asked for
    /// these settings to be kept: once they are, there is nothing left on this column to look at, and a panel
    /// that stayed would be the reader closing it a second time by hand.
    /// A refusal is the opposite - the sentence explaining it is on this panel, and dismissing the panel
    /// would take it off the screen with the fields it is about.
    /// </summary>
    private void Kept()
    {
        // Read through rather than passed in: the write reports its own answer, and a copy carried back
        // through the commit would be the same sentence held twice.
        if (_form.Unsaved.Length == 0)
        {
            _close();
            return;
        }

        Apply();
    }
}
