using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.Presets.ViewModel;

/// <summary>
/// The ways of publishing: the built-in ones, the saved ones, a name to keep the draft under, and a delete
/// per saved row.
/// A built-in preset is a promise about the picture that the backend resolves against this machine, so a row
/// can be unreachable and what applying it writes differs per machine.
/// A saved preset is a snapshot of every field, applied exactly as stored (<c>docs/presets.md</c>).
/// Both arrive already decided: the promises and their verdicts on the form, the snapshots from the store.
///
/// One state is held, the store's last answer.
/// Whether the draft is one of the presets is derived from the draft on every pass and never remembered from
/// the press that applied it, so there is no selection to reconcile after a restart.
///
/// The store is the one read on this contract that nothing announces (<c>Backend/IBackend.cs</c>,
/// <c>PresetsAsync</c>).
/// A save or a delete therefore ends in a read rather than in a patch of the list, and the re-read is offered
/// as a button, since a preset another window saved is invisible until someone asks.
///
/// Applying writes the draft and nothing else: publish settings stage until a commit carries them
/// (<c>Backend/FormSession.cs</c>, <c>WritePublish</c>).
/// </summary>
public sealed class PresetsViewModel : Observable
{
    private readonly IBackend _backend;

    /// <summary>
    /// The draft the window owns, read through on every pass.
    /// What a save sends and what each row is compared against, held nowhere here.
    /// </summary>
    private readonly FormSession _form;

    /// <summary>
    /// The running state the window owns, read through for one fact: whether the backend is being dialled.
    /// A refusal on this card is one call's, and whether another is coming is the session's
    /// (<c>Backend/Session.cs</c>).
    /// </summary>
    private readonly Session _session;

    private readonly Action<Action> _dispatch;

    /// <summary>
    /// One apply command per saved name, held so an unchanged store renders rows that compare equal.
    /// The preset is looked up at the press rather than closed over: a preset saved over keeps its name, and
    /// the row applies what the store holds then.
    /// </summary>
    private readonly Dictionary<string, DelegateCommand> _apply = [];

    /// <summary>One delete command per saved name, held for the same reason.</summary>
    private readonly Dictionary<string, PendingCommand> _delete = [];

    /// <summary>
    /// One apply command per built-in key, held for the same reason.
    /// The preset is looked up in the form the window holds at the press: a promise resolves against the
    /// draft, so the settings behind a key move as the draft does.
    /// </summary>
    private readonly Dictionary<string, DelegateCommand> _applyBuiltin = [];

    /// <summary>
    /// The store's last answer, null until the first read lands.
    /// The only state here, and written by <see cref="Took"/> alone.
    /// </summary>
    private PresetStore? _store;

    /// <summary>
    /// The backend's own sentence for the last refused call, empty otherwise.
    /// Shown as it stands (<c>docs/ipc-api.md</c>, "Errors").
    /// </summary>
    private string _refusal = "";

    /// <param name="dispatch">
    /// Hands work to the UI loop.
    /// Injected rather than reached for, so a test passes a synchronous one: an answer lands on whichever
    /// thread the transport completed on, and every property below is read by a binding.
    /// </param>
    public PresetsViewModel(IBackend backend, FormSession form, Session session, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a preset card reads and writes the store the backend keeps");
        Assert.NotNull(form, "a preset card saves the draft the window is holding");
        Assert.NotNull(session, "a preset card reads whether the backend is being dialled");
        Assert.NotNull(dispatch, "a preset card needs a UI loop to marshal an answer back to");

        _backend = backend;
        _form = form;
        _session = session;
        _dispatch = dispatch;

        Rows = [];
        Builtin = [];
        SaveCommand = new PendingCommand(() => Start(SaveAsync), dispatch, () => CanSave);
        RereadCommand = new PendingCommand(() => Start(ReadAsync), dispatch);

        // Rendered before the store is asked for, so the first list is a later pass rather than a
        // precondition.
        Apply();

        // The opening read goes through the button's own command, so there is one path into the store.
        RereadCommand.Execute(null);
    }

    // --- Input --------------------------------------------------------------------

    private string _name = "";

    /// <summary>
    /// What to save the draft under, as it is typed.
    /// A preset is selected, replaced and deleted by its name, which is why the card's one input is a name
    /// rather than a flag.
    /// </summary>
    public string Name
    {
        get => _name;
        set
        {
            if (Set(ref _name, value))
            {
                Apply();
            }
        }
    }

    // --- Outputs ------------------------------------------------------------------

    private bool _canSave;
    private string _saveLabel = "";
    private string _notice = "";
    private bool _hasNotice;
    private bool _isEmpty;
    private bool _isDialling;

    /// <summary>One row per saved preset, in the store's order.</summary>
    public ObservableCollection<PresetRow> Rows { get; }

    /// <summary>
    /// One row per built-in preset, in the form's order.
    /// Empty until the first form: which presets exist is not this shell's to know
    /// (<c>docs/ipc-api.md</c>).
    /// </summary>
    public ObservableCollection<BuiltinPresetRow> Builtin { get; }

    /// <summary>
    /// Keeps the draft's way of publishing under <see cref="Name"/>, over a preset already under it.
    /// Waits rather than going inert: the store is written across a socket, and the round trip is long enough
    /// for a second press.
    /// </summary>
    public PendingCommand SaveCommand { get; }

    /// <summary>Reads the store again, the only way a change made elsewhere reaches this card.</summary>
    public PendingCommand RereadCommand { get; }

    /// <summary>
    /// The call in flight, an already-completed task when none is.
    /// For the one caller that has to know the card has caught up with the store rather than merely asked it:
    /// a test awaits it instead of sleeping, and no render path reads it.
    /// A save and a delete each end in a read, so the reported task covers the effect and that read.
    /// </summary>
    public Task Settled { get; private set; } = Task.CompletedTask;

    public string Covers => Cards.PresetsCovers;

    public string RereadTip => Cards.PresetsReread;

    public bool CanSave { get => _canSave; private set => Set(ref _canSave, value); }

    /// <summary>
    /// Whether pressing save keeps a new preset or writes over one.
    /// Both are the same call, the name being the identity, so this word is the only warning a reader gets
    /// before a preset is replaced.
    /// </summary>
    public string SaveLabel { get => _saveLabel; private set => Set(ref _saveLabel, value); }

    /// <summary>
    /// Why the store holds fewer presets than the file did, empty while it holds all of them.
    /// The backend's own statement, carrying where the unreadable file was kept.
    /// </summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    /// <summary>
    /// Whether the store answered and holds nothing.
    /// Not the same as having no rows: a store that could not be read has none either, and
    /// <see cref="Notice"/> is that one's sentence.
    /// Neither is shown before the first answer.
    /// </summary>
    public bool IsEmpty { get => _isEmpty; private set => Set(ref _isEmpty, value); }

    public string Empty => Cards.PresetsEmpty;

    /// <summary>
    /// The backend's sentence for the last refused save, delete or read, empty otherwise.
    /// It stands beside the list rather than replacing it: a refusal is about an attempt, and the rows on
    /// screen are still the store's last answer.
    /// </summary>
    public string Refusal { get => _refusal; private set => Set(ref _refusal, value); }

    public bool HasRefusal => Refusal.Length > 0;

    /// <summary>
    /// Whether the window is still dialling behind that refusal.
    /// The refusal names one call the backend could not answer and stands until something answers, so without
    /// this the card reads the same whether the window is dialling or has stopped.
    /// False for a refusal the backend served, that one having reached a backend that is up.
    /// </summary>
    public bool IsDialling { get => _isDialling; private set => Set(ref _isDialling, value); }

    /// <summary>
    /// The one render function.
    /// Idempotent: every row is rebuilt from the store's last answer and the draft, and two passes over an
    /// unchanged pair produce rows that compare equal.
    /// </summary>
    public void Apply()
    {
        // Read through rather than held, so a save sends the same draft the rest of the flow is drawing.
        var publish = _form.Draft?.Publish;

        var saved = _store?.Saved ?? [];
        Reconcile.Onto(Rows, RowsOf(saved, publish));

        // Read through for the same reason: a preset's verdict is as old as the form it arrived on.
        Reconcile.Onto(Builtin, BuiltinRowsOf(_form.Form?.Presets ?? []));

        Notice = Statements.Of(_store?.Notice);
        HasNotice = Notice.Length > 0;
        IsEmpty = _store is not null && saved.Count == 0 && !HasNotice;

        // Trimmed as SaveAsync trims it, and asked of the store on every pass, so typing over a stored name
        // changes the word under the reader's hand rather than after the press.
        var name = Name.Trim();
        SaveLabel = saved.Any(preset => preset.Name == name) ? "Replace" : "Save";
        CanSave = name.Length > 0 && publish is not null;

        SaveCommand.Refresh();
        RereadCommand.Refresh();
        foreach (var row in Builtin)
        {
            row.Apply.Refresh();
        }

        OnPropertyChanged(nameof(HasRefusal));

        // Read off the session's verdict and not off the sentence on the card: a refusal the backend served is
        // one it was up to answer, and nothing is being dialled after it.
        IsDialling = HasRefusal && _session.Unavailable.Length > 0;

        Assert.That(Rows.Count == saved.Count, "a row per saved preset", Rows.Count, saved.Count);
        Assert.That(!IsDialling || HasRefusal, "the wait appears under the refusal it belongs to", IsDialling, Refusal);
        Assert.That(
            Builtin.Count(row => row.IsCurrent) <= 1,
            "a draft delivers at most one built-in preset's promise", Builtin.Count(row => row.IsCurrent));
        Assert.That(
            Builtin.All(row => row.IsReachable != (row.Reason.Length > 0)),
            "a built-in preset either can be applied or says why it cannot");
        Assert.That(!IsEmpty || Rows.Count == 0, "the empty sentence and the rows are never both on screen", Rows.Count);
        Assert.That(!CanSave || SaveLabel.Length > 0, "an offered save says which of the two it is", SaveLabel);
    }

    /// <summary>
    /// The rows for one reading of the store.
    /// The commands are the held ones, so an unchanged store renders rows that compare equal.
    /// </summary>
    private IReadOnlyList<PresetRow> RowsOf(IReadOnlyList<Preset> saved, PublishSettings? publish)
        => saved
            .Select(preset => new PresetRow
            {
                Name = preset.Name,
                IsCurrent = publish is not null && preset.Settings is not null && preset.Settings.Equals(publish),
                Apply = ApplyCommandOf(preset.Name),
                Delete = DeleteCommandOf(preset.Name),
            })
            .ToList();

    /// <summary>
    /// The rows for one form's built-in presets.
    /// Every word on them is this shell's, keyed by the identifier the backend sent.
    /// Every verdict on them is the backend's, rendered as it arrived.
    /// </summary>
    private IReadOnlyList<BuiltinPresetRow> BuiltinRowsOf(IReadOnlyList<BuiltinPreset> presets)
        => presets
            .Select(preset => new BuiltinPresetRow
            {
                Key = preset.Key,
                Name = Words.Preset(preset.Key),
                IsCurrent = preset.Selected,
                IsReachable = preset.Settings is not null,
                Reason = Statements.Of(preset.Reason),
                Apply = BuiltinCommandOf(preset.Key),
            })
            .ToList();

    private DelegateCommand BuiltinCommandOf(string key)
    {
        if (_applyBuiltin.TryGetValue(key, out var held))
        {
            return held;
        }

        var made = new DelegateCommand(() => UseBuiltin(key), () => BuiltinOf(key)?.Settings is not null);
        _applyBuiltin[key] = made;
        return made;
    }

    private DelegateCommand ApplyCommandOf(string name)
    {
        if (_apply.TryGetValue(name, out var held))
        {
            return held;
        }

        var made = new DelegateCommand(() => Use(name));
        _apply[name] = made;
        return made;
    }

    private PendingCommand DeleteCommandOf(string name)
    {
        if (_delete.TryGetValue(name, out var held))
        {
            return held;
        }

        var made = new PendingCommand(() => Start(() => DeleteAsync(name)), _dispatch);
        _delete[name] = made;
        return made;
    }

    /// <summary>
    /// Starts one call and makes it the one <see cref="Settled"/> reports.
    /// Every command goes through it, so one answer is in flight rather than three, and the read a save or a
    /// delete ends in stays inside the call it reports.
    /// </summary>
    private Task Start(Func<Task> call) => Settled = call();

    /// <summary>
    /// Writes one saved preset into the draft, whole.
    /// The settings come off the store's last answer rather than out of the row, so a preset saved over
    /// between the render and the press applies what it holds now.
    /// A name the store no longer carries writes nothing.
    /// </summary>
    private void Use(string name)
    {
        var preset = _store?.Saved.FirstOrDefault(saved => saved.Name == name);
        if (preset?.Settings is null)
        {
            return;
        }

        _form.WritePublish(preset.Settings);
    }

    /// <summary>
    /// Writes what the backend resolved for one built-in preset into the draft.
    /// The settings come off the form the window is holding at the press rather than out of the row: a
    /// promise resolves against the draft, so what "gaming" is here moves with the capture backend and the
    /// publish leg under it, and the row on screen may be a form old.
    /// A key the form no longer carries, or one nothing here reaches, writes nothing.
    /// Nothing is committed either way, publish settings staging until a commit carries them.
    /// </summary>
    private void UseBuiltin(string key)
    {
        var settings = BuiltinOf(key)?.Settings;
        if (settings is null)
        {
            return;
        }

        _form.WritePublish(settings);
    }

    /// <summary>One built-in preset off the form the window is holding, null for a key it does not
    /// carry.</summary>
    private BuiltinPreset? BuiltinOf(string key)
        => _form.Form?.Presets.FirstOrDefault(preset => preset.Key == key);

    /// <summary>
    /// Reads the store off the UI thread and takes the answer through the dispatcher.
    /// The one place the store is read, so the opening read and the button are one path.
    /// </summary>
    private async Task ReadAsync()
    {
        try
        {
            var store = await _backend.PresetsAsync().ConfigureAwait(false);
            _dispatch(() => Took(store, ""));
        }
        catch (BackendUnavailableException e)
        {
            // The store's last answer still stands, so the rows stay and gain the sentence saying why there
            // is no newer reading.
            _dispatch(() => Refused(e.Message));
        }
    }

    /// <summary>
    /// Keeps the draft under the typed name, then reads the store again.
    /// The read is what puts the preset on screen: whether the save landed, and under what, is the backend's
    /// answer rather than this class's.
    /// </summary>
    private async Task SaveAsync()
    {
        // Trimmed, because a reader identifies a preset by what they read: " work" and "work" are two rows
        // nobody can tell apart, and replacing one of them is a coin toss.
        var name = Name.Trim();
        var publish = _form.Draft?.Publish;

        // The pair CanSave derives from, asked again for the interval between the render and the press.
        if (name.Length == 0 || publish is null)
        {
            return;
        }

        // Copied, because the controls write the draft in place while the call is out.
        if (!await EffectAsync(() => _backend.SavePresetAsync(name, publish.Clone())).ConfigureAwait(false))
        {
            return;
        }

        _dispatch(Kept);
        await ReadAsync().ConfigureAwait(false);
    }

    /// <summary>
    /// Removes one preset, then reads the store again.
    /// A refused delete leaves the row where it is and says why, including for a name another window already
    /// deleted.
    /// The remedy is the re-read button rather than a read fired from here, which would replace a sentence
    /// nobody has read yet.
    /// </summary>
    private async Task DeleteAsync(string name)
    {
        if (!await EffectAsync(() => _backend.DeletePresetAsync(name)).ConfigureAwait(false))
        {
            return;
        }

        await ReadAsync().ConfigureAwait(false);
    }

    /// <summary>
    /// One effect on the store, with its refusal kept where the card shows it.
    /// Answers whether the store moved, which is what decides whether reading it again is worth a round trip.
    /// </summary>
    private async Task<bool> EffectAsync(Func<Task> effect)
    {
        try
        {
            await effect().ConfigureAwait(false);
            return true;
        }
        catch (BackendUnavailableException e)
        {
            _dispatch(() => Refused(e.Message));
            return false;
        }
    }

    /// <summary>
    /// One reading of the store, taken on the UI loop.
    /// The only write of <c>_store</c>.
    /// </summary>
    private void Took(PresetStore store, string refusal)
    {
        Assert.NotNull(store, "a preset read answers with what the store holds");

        _store = store;
        Refusal = refusal;
        Apply();
    }

    /// <summary>One refusal, taken on the UI loop, leaving the store's last reading standing.</summary>
    private void Refused(string reason)
    {
        Assert.That(reason.Length > 0, "a call that was refused says why");

        Refusal = reason;
        Apply();
    }

    /// <summary>
    /// Empties the name box, on the UI loop, once what was in it has been saved.
    /// Text left in it would offer to replace the preset that was saved.
    /// </summary>
    private void Kept()
    {
        Name = "";
        Refusal = "";
        Apply();
    }
}
