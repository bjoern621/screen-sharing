using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.Model;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.WatchSettings.ViewModel;

/// <summary>
/// How this machine receives: the legs a stream comes back on, the windows a receiver holds
/// packets in, and the chain a tile converts decoded frames with.
///
/// <b>It is here because this is where those settings do something.</b> They were a step of the
/// setup wizard, which is the screen for configuring what this machine <i>sends</i> - so a
/// reader setting up a broadcast walked past a page of receiving settings, and a reader who was
/// only watching had to open the broadcast wizard to change how their tiles decode. Worse, the
/// only thing that persisted them was going live: the wizard's draft reaches the backend through
/// <c>StartPublish</c>, so a reader who never published never saved a render chain. Both are the
/// same misplacement, and moving the group fixes both (<see cref="GroupPlacement"/>).
///
/// <b>The draft is not this class's.</b> It reads <see cref="FormSession"/>, which the window
/// owns and the setup wizard reads too, so the settings this panel writes and the settings a
/// publish commits are one message. A draft of its own would be the second copy that
/// <c>docs/development-principles.md</c> exists to forbid.
///
/// <b>Saving is explicit, and what it reaches is the next decode.</b> Both receive pipelines are
/// built when they are opened and neither takes a value back afterwards, so a tile already on
/// screen keeps the chain it started with - the same fact that makes <c>ApplyToStream</c> a
/// separate method on the publish side. The button says so rather than the panel pretending a
/// slider reaches a running pipeline.
///
/// <b>Outputs</b> only, written by <see cref="Apply"/> on every pass. Every write leaves through
/// the field the reader moved, into the one draft.
/// </summary>
public sealed class WatchSettingsViewModel : Observable
{
    private readonly IBackend _backend;
    private readonly FormSession _form;
    private readonly Session _session;
    private readonly Action<Action> _dispatch;

    /// <summary>
    /// What the backend said about the last save, empty otherwise. It is that side's own
    /// sentence, shown as it stands (<c>docs/ipc-api.md</c>, "Errors").
    /// </summary>
    private string _refusal = "";

    /// <param name="dispatch">
    /// Hands work to the UI loop. The answer to a save lands on whichever thread the transport
    /// completed on, and everything this writes is read by a binding that only tolerates being
    /// written from one.
    /// </param>
    public WatchSettingsViewModel(IBackend backend, FormSession form, Session session, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a watch panel asks the backend to keep what it was given");
        Assert.NotNull(form, "a watch panel draws the draft the window is holding");
        Assert.NotNull(session, "a watch panel needs the vocabulary that names its entries");
        Assert.NotNull(dispatch, "a watch panel needs a UI loop to marshal an answer back to");

        _backend = backend;
        _form = form;
        _session = session;
        _dispatch = dispatch;

        // The same generic renderer every step of the wizard uses. What the group contains and
        // which of its entries are reachable is the form's answer, unchanged by which screen
        // draws it - which is the whole reason moving the group costs no renderer.
        Group = new FieldGroupViewModel(_form.Write);

        // Persisting is an effect: it takes a round trip and the backend can refuse it. The
        // command holds whether one is in flight, which is both what the button waits on and
        // what refuses a second press.
        SaveCommand = new PendingCommand(SaveAsync, dispatch, () => _form.Draft is not null);
        SaveCommand.Changed += Apply;

        // Nothing is subscribed to here. The screen that holds this panel renders it from its own
        // pass, which is the arrangement every other child component on this shell has: one
        // notification, one render, and no two components rendering the same change twice.
        Apply();
    }

    // --- Outputs ------------------------------------------------------------------

    private string _notice = "";
    private bool _hasNotice;

    /// <summary>The controls, drawn by the one generic renderer the wizard's steps use.</summary>
    public FieldGroupViewModel Group { get; }

    /// <summary>Keeps these settings for the decodes this machine opens next.</summary>
    public PendingCommand SaveCommand { get; }

    /// <summary>What the button says it does, which is the half of it a label cannot carry.</summary>
    public string SaveTip =>
        "Keeps these settings. A tile already on screen keeps the pipeline it was opened with, so a change reaches the next decode rather than the running one.";

    /// <summary>What the backend said about the last save, empty otherwise.</summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    /// <summary>
    /// The one render function. Safe to run twice: the converge it asks for is skipped when the
    /// draft has not moved, and the group's own pass produces fields that compare equal, so an
    /// unchanged pass fires no binding.
    /// </summary>
    public void Apply()
    {
        // Reconciled from the render pass rather than performed by it, exactly as the wizard
        // does it: the pass states that it wants a form, and the converge decides whether
        // anything has to be asked (docs/development-principles.md, "Idempotency").
        _form.Sync();

        var form = _form.Form;
        Group.Apply(GroupOf(form), _session.Words, form?.Settings);

        Notice = _refusal;
        HasNotice = Notice.Length > 0;
        SaveCommand.Refresh();

        Assert.That(HasNotice == (Notice.Length > 0), "the notice and its sentence agree", Notice);
        Assert.That(
            Group.IsResolved || Group.Fields.Count == 0,
            "a panel with no group draws no controls", Group.Fields.Count);
    }

    /// <summary>
    /// The watch group of the resolved form, or null where the form carries none. Looked up by
    /// the key that says which screen draws it, so this panel and the wizard's own filter read
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
    /// Persists the draft. It hands over a copy for the reason the publish commit does: the
    /// controls write the draft in place, so passing the live instance would let a keystroke
    /// change the message while it is being sent.
    ///
    /// <b>Safe to press twice.</b> The call names a state - these are the held settings - so a
    /// second press with nothing changed asks for a state that already holds, which is a success
    /// (<c>docs/development-principles.md</c>, "Effects across a process boundary").
    /// </summary>
    private async Task SaveAsync()
    {
        var draft = Assert.NotNull(_form.Draft, "a save that was offered was drawn from a draft");
        var settings = draft.Clone();

        try
        {
            await _backend.SaveSettingsAsync(settings).ConfigureAwait(false);
            _dispatch(() => Answered(""));
        }
        catch (BackendUnavailableException e)
        {
            _dispatch(() => Answered(e.Message));
        }
        catch (OperationCanceledException)
        {
            // Nothing cancels this call, since it carries no token. A transport that reports one
            // anyway still has to leave the button pressable rather than locked forever.
            _dispatch(() => Answered(""));
        }
    }

    /// <summary>
    /// Records what the backend said and re-renders, on the UI loop. An empty reason is a
    /// success, which clears whatever the last failure left - the render function's usual
    /// property applied to a string.
    /// </summary>
    private void Answered(string reason)
    {
        _refusal = reason;
        Apply();
    }
}
