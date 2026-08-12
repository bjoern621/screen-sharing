using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.Presets.ViewModel;

/// <summary>
/// The ways of publishing: the built-in ones this machine can reach, what the store holds, a
/// name to keep the draft under, and a way to take one back out.
///
/// <b>The two kinds on this card are not two lists of the same thing.</b> A built-in preset is a
/// promise about the picture, and which encoder, pixel format and capture backend deliver it
/// here is a search the backend ran against this machine - so a row can be unreachable, and what
/// applying it writes differs from machine to machine. A saved preset is a snapshot of every
/// field under a name the user typed, applied exactly as it was stored. Both arrive already
/// decided: the promises and their verdicts ride on the form, the snapshots come from the store
/// (<c>docs/presets.md</c>).
///
/// <b>It holds one state, and that is the store's last answer.</b> Which presets exist is the
/// backend's, the settings inside each are the backend's, and whether the draft is one of them
/// is derived on every pass from the draft itself - never remembered from the press that applied
/// it. So there is no selection here to fall out of step with the settings, and a restart brings
/// the same card back without anything having been reconciled.
///
/// <b>The store is the one read on this contract that nothing announces.</b> Presets are a file
/// rather than state the backend runs on, so no event says one appeared or went
/// (<c>Backend/IBackend.cs</c>, <c>PresetsAsync</c>). Two consequences are on this card. A save
/// or a delete is followed by a read rather than by patching the list with what was just sent,
/// which is what keeps the rows the store's answer and not this class's guess. And the reader
/// gets the re-read as a button, because a second window's save is invisible here until someone
/// asks again.
///
/// <b>Applying writes the draft and nothing else.</b> A preset is a way of publishing, and
/// publish settings are staged until a commit carries them, so trying one out costs nothing and
/// puts nothing on the air (<c>Backend/FormSession.cs</c>, <c>WritePublish</c>).
/// </summary>
public sealed class PresetsViewModel : Observable
{
    private readonly IBackend _backend;

    /// <summary>
    /// The draft, owned by the window and read through on every pass: it is what a save sends,
    /// and what each row is compared against to say whether it is the one in force. This class
    /// holds no copy of it.
    /// </summary>
    private readonly FormSession _form;

    private readonly Action<Action> _dispatch;

    /// <summary>
    /// One apply command per preset name, made once and reused, so an unchanged store renders
    /// rows that compare equal and the bound list is left alone. The command looks the preset up
    /// by name when it is pressed rather than closing over the settings it was made with - a
    /// preset saved over keeps its name, and the row must then apply what the store holds now.
    /// </summary>
    private readonly Dictionary<string, DelegateCommand> _apply = [];

    /// <summary>One delete command per preset name, held for the same reason.</summary>
    private readonly Dictionary<string, PendingCommand> _delete = [];

    /// <summary>
    /// One apply command per built-in preset, held for the reason the saved ones are. It looks
    /// the preset up in the form the window holds now rather than closing over the settings the
    /// row was rendered with: a preset resolves against the draft, so the settings behind a key
    /// move as the draft does.
    /// </summary>
    private readonly Dictionary<string, DelegateCommand> _applyBuiltin = [];

    /// <summary>
    /// The last answer the store gave, and null until the first read lands. It is the only state
    /// this class holds and is written in one place, by <see cref="Took"/>.
    /// </summary>
    private PresetStore? _store;

    /// <summary>
    /// What the backend said when it refused the last call, empty otherwise. It is that side's
    /// own sentence, shown as it stands (<c>docs/ipc-api.md</c>, "Errors").
    /// </summary>
    private string _refusal = "";

    /// <param name="dispatch">
    /// Hands work to the UI loop. Injected rather than reached for, so this type stays free of a
    /// toolkit and a test can pass a synchronous dispatcher: an answer arrives on whichever
    /// thread the transport completed on, and every property below is read by a binding.
    /// </param>
    public PresetsViewModel(IBackend backend, FormSession form, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a preset card reads and writes the store the backend keeps");
        Assert.NotNull(form, "a preset card saves the draft the window is holding");
        Assert.NotNull(dispatch, "a preset card needs a UI loop to marshal an answer back to");

        _backend = backend;
        _form = form;
        _dispatch = dispatch;

        Rows = [];
        Builtin = [];
        SaveCommand = new PendingCommand(() => Start(SaveAsync), dispatch, () => CanSave);
        RereadCommand = new PendingCommand(() => Start(ReadAsync), dispatch);

        // Rendered before the store is asked for, so the card is complete whether or not the
        // backend answers, and the first list is a later pass rather than a precondition.
        Apply();

        // The opening read goes through the same command the button presses, so what a reader
        // sees while the card fills itself is what they see when they refill it by hand, and
        // there is one path into the store rather than two.
        RereadCommand.Execute(null);
    }

    // --- Input --------------------------------------------------------------------

    private string _name = "";

    /// <summary>
    /// What to save the draft under, as it is being typed. The one input on this card, and the
    /// reason the switch it replaced could never work: a preset is selected, replaced and
    /// deleted by its name, and a toggle has none to give.
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

    /// <summary>One row per preset the store holds, in the order it holds them.</summary>
    public ObservableCollection<PresetRow> Rows { get; }

    /// <summary>
    /// One row per built-in preset, in the order the backend offers them. It is empty until the
    /// first form arrives, and it is the form's own list: which presets exist is not this
    /// shell's to know (<c>docs/ipc-api.md</c>).
    /// </summary>
    public ObservableCollection<BuiltinPresetRow> Builtin { get; }

    /// <summary>
    /// Keeps the draft's way of publishing under <see cref="Name"/>, replacing a preset already
    /// saved under it. It waits rather than going inert, because the store is written across a
    /// socket and the round trip is long enough for a reader to press again.
    /// </summary>
    public PendingCommand SaveCommand { get; }

    /// <summary>Reads the store again, which is the only way a change made elsewhere reaches this card.</summary>
    public PendingCommand RereadCommand { get; }

    /// <summary>
    /// The call in flight, and an already-completed task when none is. It is the seam's timing
    /// made observable, for the one caller that legitimately needs it: something that has to know
    /// the card has caught up with the store rather than merely having asked it. A test waits on
    /// it instead of sleeping; nothing in a render path touches it.
    ///
    /// A save and a delete each end in a read, so the task this reports covers the effect and the
    /// reading that follows it - which is the whole of what "the store has answered" means here.
    /// </summary>
    public Task Settled { get; private set; } = Task.CompletedTask;

    /// <summary>What a preset covers, and what it deliberately leaves where it is.</summary>
    public string Covers => Cards.PresetsCovers;

    /// <summary>Why the list can be behind, on the button that answers it.</summary>
    public string RereadTip => Cards.PresetsReread;

    /// <summary>Whether the name names something to save, and there is a draft to save under it.</summary>
    public bool CanSave { get => _canSave; private set => Set(ref _canSave, value); }

    /// <summary>
    /// What pressing save will do: keep a new preset, or write over the one already under that
    /// name. Both are the same call - the name is the identity, so saving over a preset is how
    /// one is edited - and the word is the only warning a reader gets before it happens.
    /// </summary>
    public string SaveLabel { get => _saveLabel; private set => Set(ref _saveLabel, value); }

    /// <summary>
    /// Why the store holds fewer presets than it did, empty when it holds all of them. It is the
    /// backend's own statement, rendered here, and it carries where the unreadable file was kept.
    /// </summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    /// <summary>
    /// Whether the store answered and holds nothing. It is not the same as having no rows: a
    /// store that could not be read has no rows either, and <see cref="Notice"/> is the sentence
    /// for that one. Before the first answer neither is shown, because nothing is known yet.
    /// </summary>
    public bool IsEmpty { get => _isEmpty; private set => Set(ref _isEmpty, value); }

    /// <summary>The sentence for a store that answered and holds nothing.</summary>
    public string Empty => Cards.PresetsEmpty;

    /// <summary>
    /// What the backend said when it refused the last save, delete or read, empty otherwise. A
    /// refusal is about an attempt rather than about a state, so it stands beside the list rather
    /// than replacing it: the rows on screen are still the last ones the store answered with.
    /// </summary>
    public string Refusal { get => _refusal; private set => Set(ref _refusal, value); }

    public bool HasRefusal => Refusal.Length > 0;

    /// <summary>
    /// The one render function. Idempotent: every row is rebuilt from the store's last answer
    /// and the draft, and two passes over an unchanged pair produce rows that compare equal.
    /// </summary>
    public void Apply()
    {
        // Read through rather than held: what a save sends and what a row is compared against
        // are the same draft the rest of the flow is drawing.
        var publish = _form.Draft?.Publish;

        var saved = _store?.Saved ?? [];
        Reconcile.Onto(Rows, RowsOf(saved, publish));

        // The form's own answer, read through on every pass like the draft above it: a preset is
        // resolved against the settings, so its verdict is as old as the form it came on.
        Reconcile.Onto(Builtin, BuiltinRowsOf(_form.Form?.Presets ?? []));

        Notice = Statements.Of(_store?.Notice);
        HasNotice = Notice.Length > 0;
        IsEmpty = _store is not null && saved.Count == 0 && !HasNotice;

        // The name as it would be saved, which is the trimmed one (SaveAsync says why). Asked of
        // the store on every pass, so typing over a name already in the list changes the word on
        // the button under the reader's hand rather than after the press.
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

        Assert.That(Rows.Count == saved.Count, "a row per saved preset", Rows.Count, saved.Count);
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
    /// The rows for one reading of the store. The commands are the held ones, so an unchanged
    /// store renders rows that compare equal.
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
    /// The rows for one form's built-in presets. Every word on them is this shell's, keyed by the
    /// identifier the backend sent; every verdict on them is the backend's, rendered as it
    /// arrived.
    /// </summary>
    private IReadOnlyList<BuiltinPresetRow> BuiltinRowsOf(IReadOnlyList<BuiltinPreset> presets)
        => presets
            .Select(preset => new BuiltinPresetRow
            {
                Key = preset.Key,
                Name = Words.Preset(preset.Key),
                Promise = Descriptions.Preset(preset.Key),
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
    /// Starts one call and makes it the one <see cref="Settled"/> reports. Every command here
    /// goes through it, so what this card has in flight is one answer rather than three - and the
    /// reads a save or a delete ends in do not replace it, being inside the call it reports.
    /// </summary>
    private Task Start(Func<Task> call) => Settled = call();

    /// <summary>
    /// Writes one preset into the draft, whole.
    ///
    /// The settings come off the store's last answer rather than out of the row, so a preset
    /// saved over between the render and the press applies what it holds now. A name the store
    /// no longer carries has no row on screen, and the command left over from the one it had
    /// does nothing rather than applying a preset that is gone.
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
    ///
    /// The settings come off the form the window holds now rather than out of the row, because a
    /// preset resolves against the draft: what "gaming" is on this machine changes as the capture
    /// backend or the publish leg under it changes, and the row on screen may be a form old. A
    /// preset the form no longer carries, or one nothing here reaches, writes nothing rather than
    /// applying a way of publishing this machine cannot run.
    ///
    /// Nothing is committed by it. Publish settings are staged until a commit carries them, so
    /// trying a preset out costs nothing and puts nothing on the air.
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

    /// <summary>One built-in preset off the form the window holds now, and null for a key it does
    /// not carry.</summary>
    private BuiltinPreset? BuiltinOf(string key)
        => _form.Form?.Presets.FirstOrDefault(preset => preset.Key == key);

    /// <summary>
    /// Reads the store, off the UI thread, and takes the answer through the dispatcher. The one
    /// place the store is read, so the opening read and the button are one path.
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
            // The last answer the store gave is still the last answer it gave, so the rows stay
            // and gain the sentence saying why there is no newer reading.
            _dispatch(() => Refused(e.Message));
        }
    }

    /// <summary>
    /// Keeps the draft under the typed name, then reads the store again.
    ///
    /// The read is what puts the new preset on screen. Adding a row from what was sent would be
    /// this class stating what the store holds, which is the one thing it does not know: the
    /// backend decides whether the save landed and under what.
    /// </summary>
    private async Task SaveAsync()
    {
        // Trimmed, because the list identifies a preset by a name a reader reads: " work" and
        // "work" would be two rows nobody could tell apart, and replacing one of them would be a
        // coin toss.
        var name = Name.Trim();
        var publish = _form.Draft?.Publish;

        // The pair CanSave is derived from, asked again at the press. The command is gated on it
        // already; this is the guard for the interval between the render and the press.
        if (name.Length == 0 || publish is null)
        {
            return;
        }

        // The draft is copied because the controls write it in place, and the call is in flight
        // for as long as the round trip lasts.
        if (!await EffectAsync(() => _backend.SavePresetAsync(name, publish.Clone())).ConfigureAwait(false))
        {
            return;
        }

        _dispatch(Kept);
        await ReadAsync().ConfigureAwait(false);
    }

    /// <summary>
    /// Removes one preset, then reads the store again.
    ///
    /// A refused delete leaves the row where it is and says why. That includes the one the
    /// backend answers when the name is already gone - another window deleted it - and the
    /// remedy for that one is the re-read, which is a button rather than something done here:
    /// a read fired from a failure would replace the sentence the reader has not read yet.
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
    /// One effect on the store, with its refusal kept where the card shows it. It answers
    /// whether the store moved, which is what decides whether reading it again is worth a round
    /// trip.
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

    /// <summary>Takes one reading of the store, on the UI loop. The only write of <c>_store</c>.</summary>
    private void Took(PresetStore store, string refusal)
    {
        Assert.NotNull(store, "a preset read answers with what the store holds");

        _store = store;
        Refusal = refusal;
        Apply();
    }

    /// <summary>Takes one refusal, on the UI loop, leaving the last reading of the store standing.</summary>
    private void Refused(string reason)
    {
        Assert.That(reason.Length > 0, "a call that was refused says why");

        Refusal = reason;
        Apply();
    }

    /// <summary>
    /// Empties the name box, on the UI loop, once what was in it has been saved. The box has
    /// done its job, and text left in it would offer to replace a preset that was just written.
    /// </summary>
    private void Kept()
    {
        Name = "";
        Refusal = "";
        Apply();
    }
}
