using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Backend;

/// <summary>
/// Settings draft being edited, and the last form the backend resolved it to.
///
/// <b>One owner of both, for the whole window.</b>
/// The setup wizard configures what this machine sends and the viewer how it receives;
/// a draft each would be two copies of one fact, the wizard's commit persisting the whole message
/// and overwriting watch settings its copy never saw (<c>docs/development-principles.md</c>, "Stateless").
///
/// Sibling of <see cref="Session"/>: the session owns the running state the backend reports,
/// this owns the settings nobody has committed.
/// Neither derives anything from the other.
///
/// <b>Decides nothing.</b>
/// Which controls exist, which values they offer, which are greyed and why, what the configuration costs
/// and whether it can be published all arrive decided from <see cref="IBackend.ResolveFormAsync"/>
/// (<c>docs/ipc-api.md</c>, "The rule").
///
/// <b>The resolve is a round trip, and that shapes the class.</b>
/// The backend answers over a socket, so a render pass that waited would freeze the window it is drawing.
/// The last form answered with is <b>explicit state</b>, every reader <b>reads it continuously</b>
/// and awaits nothing, and a draft change starts a resolve whose answer lands later and raises <see cref="Changed"/>.
/// A window with no form yet is a state rather than a gap.
///
/// Three properties make that safe on every keystroke.
/// <see cref="Sync"/> is <b>idempotent</b>: a draft still equal to the one last asked about asks nothing,
/// so a render pass reconciles unconditionally.
/// <b>One resolve is out at a time</b>, and the answer asks for whatever the draft has become since:
/// a slider dragged across its range writes a step per pointer move and costs a round trip per answer
/// rather than one per step.
/// Each answer describes a value the reader passed through, which is what prices a control as it is moved.
/// And <b>the latest answer wins</b>: each resolve carries a request number,
/// an answer nothing is waiting for is dropped rather than drawn over a newer draft's form,
/// and an answer about a draft the reader has moved off is drawn without being taken back into the draft.
///
/// <see cref="Persist"/> holds the same shape for the write half, and for the same reason.
/// </summary>
public sealed class FormSession
{
    private readonly IBackend _backend;

    /// <summary>Read for one edge only: the backend answering again after it could not be reached.</summary>
    private readonly Session _session;

    private readonly Action<Action> _dispatch;

    /// <summary>
    /// Last form the backend answered with. Null until the first answer lands.
    /// State every reader draws from, written by <see cref="Adopt"/> and nowhere else.
    /// </summary>
    private Form? _form;

    /// <summary>
    /// Settings being edited.
    /// Null until the stored settings arrive.
    /// Never read for meaning: a value goes in and the answer that comes back is what is drawn.
    /// Written in place by <see cref="Write"/>, so nothing else holds this instance: the backend is handed a copy
    /// and the form keeps its own.
    /// </summary>
    private Settings? _draft;

    /// <summary>
    /// Draft the backend was last asked about: the copy handed to the resolve,
    /// replaced by the settings its answer carried.
    /// Never mutated once set, so comparing the draft against it says whether anything moved,
    /// which is the whole round-trip guard.
    /// </summary>
    private Settings? _asked;

    /// <summary>
    /// The stream in force when the draft was last handed over, null with none.
    /// The answer is a function of the draft and of that stream (<c>form.proto</c>, <c>Form.in_force</c>),
    /// so both are inputs to the round-trip guard: a stream that started or ended under an unchanged draft
    /// is asked about, and an event under which neither moved is not.
    /// The live message alone, so a backend with nothing publishing reads as the same input every time.
    /// </summary>
    private PublishState.Types.Live? _askedUnder;

    /// <summary>
    /// Settings the backend is holding. Null until the opening read answers.
    /// A different fact from the draft, and the difference is what a staged group means: effects are served off
    /// the backend's own settings rather than off anything a call hands over, a decode reading its render chain
    /// and both jitter buffers out of them (<c>docs/ipc-api.md</c>).
    /// Equal to the draft until something is edited, equal again once it is kept.
    /// </summary>
    private Settings? _stored;

    /// <summary>
    /// Which resolve is being waited for, counting up.
    /// An older number belongs to a draft the reader has moved off, and is dropped.
    /// </summary>
    private int _request;

    /// <summary>Set per resolve, cancelled when a newer draft supersedes it.</summary>
    private CancellationTokenSource? _cancel;

    /// <summary>Held while a resolve is out, so two never overlap.</summary>
    private bool _resolving;

    /// <summary>Held while the reader has a control's thumb, so a repair waits for the release.</summary>
    private bool _sweeping;

    /// <summary>
    /// Completed once the run of resolves a draft change started has caught up with the draft. Null while
    /// nothing is out.
    /// One source per run, as <see cref="_written"/> is: every caller on it waits for the same thing, the form
    /// on screen describing what the reader is holding.
    /// </summary>
    private TaskCompletionSource? _caught;

    /// <summary>
    /// Settings waiting to be persisted. Null when none are.
    /// Replaced rather than queued: a newer draft says everything an older one did.
    /// </summary>
    private Settings? _pending;

    /// <summary>Held while a write is out, so two never overlap.</summary>
    private bool _persisting;

    /// <summary>
    /// Completed once the run a write joined has drained. Null while nothing is queued.
    /// One source per run: every caller on it waits for the same thing, the newest draft stored.
    /// </summary>
    private TaskCompletionSource? _written;

    /// <summary>
    /// Whether the session last reported the backend absent.
    /// Held to tell the moment a backend came back from the state of it being there, so a recovery is asked
    /// about once rather than on every event after it.
    /// </summary>
    private bool _backendWasAbsent;

    /// <param name="dispatch">
    /// Hands work to the UI loop.
    /// Injected rather than reached for, so no toolkit is bound in here and a test can pass a synchronous
    /// dispatcher, the arrangement <see cref="Session"/> uses too.
    /// A resolve answers on whichever thread the transport completed on, and every reader writes bound
    /// properties off it.
    /// </param>
    public FormSession(IBackend backend, Session session, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a draft needs the backend that describes it");
        Assert.NotNull(session, "a draft watches the running state for a backend that came back");
        Assert.NotNull(dispatch, "a draft needs a UI loop to marshal an answer back to");

        _backend = backend;
        _session = session;
        _dispatch = dispatch;

        // What the backend would answer has moved, the encoder probe landing being what raises it, so this
        // reads again rather than changing anything held here.
        // Marshalled first: the signal arrives on whichever thread the transport completed on.
        _backend.Changed += () => _dispatch(Reask);

        // Watched for one edge: the backend answering again after it could not be reached.
        // Raised on the UI loop by the session, so there is nothing to marshal.
        _session.Changed += OnSessionChanged;

        // The settings this window holds are a copy of the backend's, and this is where news that they moved
        // arrives. Raised on the UI loop by the session as well.
        _session.SettingsMoved += Reread;

        // The one read with no draft in front of it.
        // The stored settings are what this opens on and nothing here derives them, so it is started once rather
        // than reconciled from a render pass the way every resolve after it is.
        Start(draft: null);
    }

    /// <summary>
    /// Raised on the UI loop after the form, the draft or the reason there is no newer form moved.
    /// Carries nothing: the form is whole and read through, so what it changed to and that it changed are two
    /// facts and only the second belongs on a signal.
    /// </summary>
    public event Action? Changed;

    /// <summary>
    /// Last form the backend answered with. Null until the first answer lands.
    /// Read through per render pass, never copied: a cached form draws an older answer than the one the draft
    /// resolves to.
    /// </summary>
    public Form? Form => _form;

    /// <summary>
    /// Settings being edited. Null until the stored settings arrive.
    /// A commit copies it: the controls write this instance in place, so the live one would let a keystroke
    /// change the settings mid-send.
    /// </summary>
    public Settings? Draft => _draft;

    /// <summary>
    /// Settings the backend is holding.
    /// Null until the opening read answers.
    /// Read where a call has to name a value the backend will act on rather than one the reader is looking at.
    /// The leg a decode opens on is the case that exists:
    /// the backend reads that decode's other knobs out of these same settings,
    /// so naming the draft's leg would run half a panel's choices and hold the rest back.
    /// </summary>
    public Settings? Stored => _stored;

    /// <summary>
    /// Whether the form on screen answers for the draft as it stands,
    /// which is the two messages carrying the same settings.
    /// False from a write until the answer about it lands,
    /// and that window is what a control checks before taking a value off the form:
    /// what the form carries there is what the reader held a round trip ago,
    /// and assigning it would put a thumb back under the pointer or unmark a card that was just clicked.
    /// A repair therefore reaches a control on the answer that carries it.
    ///
    /// Compared against the answer rather than against the draft last handed over,
    /// those two being equal from the moment a resolve starts and saying nothing about what is drawn.
    /// </summary>
    public bool IsAnswered => _form is not null && _draft is not null && _draft.Equals(_form.Settings);

    /// <summary>
    /// Why the last read could not be answered, empty while the backend is answering.
    /// The backend's own sentence, shown as it stands.
    /// Cleared by the next answer, so a recovered backend clears the notice a failed read left rather than
    /// leaving it under a form that is drawing again.
    /// </summary>
    public string Unavailable { get; private set; } = "";

    /// <summary>
    /// Why the last write could not be stored, empty while they are being stored.
    /// The backend's own sentence, shown as it stands.
    ///
    /// Separate from <see cref="Unavailable"/>: neither stands in for the other.
    /// An unanswered read leaves an older answer on screen and a publish with nothing to go on.
    /// An unstored write leaves the screen showing what the reader typed while the backend runs on the value
    /// before it, so the setting is worth naming and the publish is not worth blocking.
    /// </summary>
    public string Unsaved { get; private set; } = "";

    /// <summary>
    /// Read in flight, a completed task when none is.
    /// For the caller that has to know the screen caught up with the draft rather than was merely asked to:
    /// a test waits on it instead of sleeping, and no render path touches it.
    /// Never faults on a cancellation, a cancelled resolve being one this class asked for.
    /// </summary>
    public Task Settled { get; private set; } = Task.CompletedTask;

    /// <summary>
    /// Asks for the form this draft resolves to, unless the backend has already been asked for it.
    /// <b>Idempotent, which makes it safe on a render pass.</b>
    /// The resolve is side-effect free and answers the same form for the same draft under the same stream
    /// (<c>docs/ipc-api.md</c>), so a draft still equal to the one last handed over,
    /// with what publishes unchanged since, has nothing to learn from a second round trip, landed or in flight.
    /// A hundred render passes cost the one call.
    /// </summary>
    public void Sync()
    {
        // No draft yet means the read that fetches the stored settings is still out, started by the constructor.
        if (_draft is null || (_draft.Equals(_asked) && Equals(_session.Publish?.Live, _askedUnder)))
        {
            return;
        }

        // The answer out there asks for this draft when it lands (Settle), so a burst of writes costs the round
        // trip it arrived in and one more.
        if (_resolving)
        {
            return;
        }

        // Copied because the controls write the draft in place: the live instance would let the next keystroke
        // change the message mid-send, and leave the answer describing settings nobody asked about.
        var draft = _draft.Clone();
        _asked = draft;
        _askedUnder = _session.Publish?.Live;
        Start(draft);
    }

    /// <summary>
    /// States whether the reader is holding a control's thumb.
    /// A sweep writes a value per pointer move and every one of them is asked about,
    /// the round-trip guard above coalescing the burst into one question per answer,
    /// so every figure the form carries is priced for the value under the thumb
    /// (<c>docs/settings-editing.md</c>).
    ///
    /// <b>Names the state rather than the edge</b>, so a control reporting the same one twice changes nothing and
    /// a widget that lost the pointer can say so without knowing whether it already had
    /// (<c>docs/development-principles.md</c>, "Idempotency").
    ///
    /// What it holds back is the repair: a draft the backend walked to a legal value is adopted on the release,
    /// taking it mid-gesture being what would drag the thumb out from under the pointer.
    /// </summary>
    public void Sweeping(bool sweeping)
    {
        _sweeping = sweeping;

        if (!sweeping)
        {
            AdoptHeld();
            Sync();
            Announce();
        }
    }

    /// <summary>
    /// Takes the repair the last answer carried and the sweep held back.
    /// The answer describes the draft as it stands,
    /// so nothing here moves the reader off a value they are still holding:
    /// a newer draft means a resolve is out, and its own answer lands with the repair.
    /// </summary>
    private void AdoptHeld()
    {
        if (_resolving || _form is null || _draft is null || !_draft.Equals(_asked))
        {
            return;
        }

        _draft = _form.Settings.Clone();
        _asked = _form.Settings;
    }

    /// <summary>
    /// One field write, from whichever control the reader moved.
    /// The whole draft re-resolves: which other controls that frees or greys is the backend's answer, and asking
    /// is cheap by contract.
    /// </summary>
    public void Write(string key, FieldValue value)
    {
        // A control the reader can move was drawn from a form, and a form was resolved from a draft.
        var draft = Assert.NotNull(_draft, "a control the reader moved was drawn from a draft");
        Assert.That(key.Length > 0, "a write names the settings field it changes");

        // A write to a field the followed preset decides takes the value into the reader's own hands,
        // so the draft stops following with the edit and keeps the resolved values it was showing.
        // Which fields those are is read off the form, never decided here
        // (form.proto, Form.preset_owned_field_keys).
        if (PresetOwns(key))
        {
            draft.Publish.Preset = "";
        }

        SettingsDraft.Write(draft, key, value);
        Sync();

        // An applied group's field is the setting itself rather than a proposal a commit turns into one, so it
        // is stored as it is written (form.proto, FieldGroup.applied).
        // Which groups those are is read off the form, never decided here.
        if (Applies(key))
        {
            _ = Persist(draft);
        }

        Announce();
    }

    /// <summary>
    /// Stores the draft as it stands, and answers once the write has landed.
    ///
    /// What a staged group's commit runs: nothing in such a group reaches the backend as it is edited,
    /// so a screen drawing one needs a way to say "these, now".
    /// The write is the whole settings message either way,
    /// so it goes down the queue an applied field's keystroke uses: two unary calls carry no ordering between them,
    /// and the older snapshot landing last is what <see cref="Persist"/> exists to prevent.
    ///
    /// <b>Safe to run twice.</b>
    /// It names a state, that these are the stored settings,
    /// so a second run with nothing changed asks for a state that already holds
    /// (<c>docs/development-principles.md</c>, "Effects across a process boundary").
    /// Whether it landed is <see cref="Unsaved"/>; this answers when the attempt is over either way.
    /// </summary>
    public Task SaveAsync()
    {
        // A commit is offered beside a form, and a form was resolved from a draft.
        var draft = Assert.NotNull(_draft, "a save that was offered was drawn from a draft");

        return Persist(draft);
    }

    /// <summary>
    /// Replaces the way of publishing whole, which is what applying a preset is.
    /// The draft's other two groups are untouched: a preset is a <c>PublishSettings</c> and nothing else, so
    /// where the relay is and how this machine watches are not its to say (<c>docs/presets.md</c>).
    ///
    /// <b>An assignment and not a merge</b>, which is why the settings travel whole (<c>settings.proto</c>,
    /// <c>Preset</c>).
    /// Merged, what a preset produced would depend on what the form happened to hold first, and one preset would
    /// mean different pictures on two machines.
    /// The resolve answers with the repaired version of what was assigned, the adoption every other write ends
    /// in.
    ///
    /// The one write that names a settings group.
    /// Every other addresses a field by the key the form gave it (<see cref="Write"/>), and no key names a whole
    /// group, nor should one, since no control writes one.
    /// Naming it is honest here because the contract names it too: a preset is defined as that group.
    /// </summary>
    public void WritePublish(PublishSettings publish)
    {
        Assert.NotNull(publish, "applying a preset needs the way of publishing it saved");

        // A preset is applied to a form the reader is looking at, and a form was resolved from a draft.
        var draft = Assert.NotNull(_draft, "a preset the reader applied was offered beside a draft");

        // Copied because the store holds its message for as long as the list is on screen:
        // assigning the instance would let the next keystroke edit the preset.
        draft.Publish = publish.Clone();
        Sync();

        // The question a field write asks, asked about every field this one moved: settings themselves,
        // or a proposal a commit turns into settings?
        // Publish settings are staged, so this stores nothing
        // and a preset being tried out is not what the next stream starts on.
        // Read off the form rather than stated here, so a group that becomes applied is stored by this write too.
        if (AppliesToGroup(SettingsDraft.PublishGroup))
        {
            _ = Persist(draft);
        }

        Announce();
    }

    /// <summary>
    /// Puts one group of settings back to what a fresh installation holds.
    ///
    /// <b>The values are the form's, stated per field</b> (<c>form.proto</c>, <c>Field.default_value</c>).
    /// What a setting starts as is the same fact as which values it may take, and both are the backend's
    /// (<c>docs/ipc-api.md</c>, "The rule"), so a group that gains a field is one this puts back with nothing
    /// here to edit.
    ///
    /// <b>One write, not one per field.</b>
    /// The whole group reaches the draft before anything is asked or stored, so the resolve sees the whole reset
    /// and an applied group is stored once.
    /// A reset is one change of mind about a group rather than a burst of writes a reader could have made by hand.
    /// </summary>
    public void Reset(string groupKey)
    {
        Assert.That(groupKey.Length > 0, "a reset names the group it puts back");

        // A heading the reader pressed came from a form, and a form was resolved from a draft.
        var draft = Assert.NotNull(_draft, "a reset the reader asked for was offered beside a draft");
        var group = Assert.NotNull(GroupOf(groupKey), "a reset names a group the form carries");

        foreach (var field in group.Fields)
        {
            var value = Assert.NotNull(field.DefaultValue, "a field the form carries states what it starts as");

            SettingsDraft.Write(draft, field.Key, value);
        }

        Sync();

        if (group.Applied)
        {
            _ = Persist(draft);
        }

        Announce();
    }

    private FieldGroup? GroupOf(string groupKey)
    {
        if (_form is null)
        {
            return null;
        }

        foreach (var group in _form.Groups)
        {
            if (group.Key == groupKey)
            {
                return group;
            }
        }

        return null;
    }

    /// <summary>
    /// Whether a write to this field is the setting itself, which the form states per group (<c>form.proto</c>,
    /// <c>FieldGroup.applied</c>).
    /// False for a key no drawn group carries, and before the first form lands:
    /// what a field means arrives from the backend,
    /// so a write it has said nothing about is held rather than stored on a guess.
    /// </summary>
    private bool Applies(string key)
    {
        if (_form is null)
        {
            return false;
        }

        foreach (var group in _form.Groups)
        {
            foreach (var field in group.Fields)
            {
                if (field.Key == key)
                {
                    return group.Applied;
                }
            }
        }

        return false;
    }

    /// <summary>
    /// Whether the followed preset decides this field, which the form states as a key list
    /// (<c>form.proto</c>, <c>Form.preset_owned_field_keys</c>).
    /// False for a detached draft, whose list is empty, and before the first form lands,
    /// so a write nothing has described never drops a followed preset on a guess.
    /// </summary>
    private bool PresetOwns(string key)
    {
        return _form is not null && _form.PresetOwnedFieldKeys.Contains(key);
    }

    /// <summary>
    /// Whether a write to any field of one settings group is the setting itself.
    /// Not the same question as <see cref="Applies"/>: the form groups the screen by what the reader is deciding
    /// and a key by which message holds the value,
    /// so one settings group's fields reach the screen spread over several form groups
    /// (<c>backend/internal/form/keys.go</c>).
    /// Any one of them being applied makes the write a setting,
    /// that being the field the backend would otherwise never be handed.
    /// </summary>
    private bool AppliesToGroup(string group)
    {
        Assert.That(group.Length > 0, "asking whether a group is applied names the group");

        if (_form is null)
        {
            return false;
        }

        var prefix = group + SettingsDraft.KeySeparator;
        foreach (var drawn in _form.Groups)
        {
            if (!drawn.Applied)
            {
                continue;
            }

            foreach (var field in drawn.Fields)
            {
                if (field.Key.StartsWith(prefix, StringComparison.Ordinal))
                {
                    return true;
                }
            }
        }

        return false;
    }

    /// <summary>
    /// Queues the draft to be stored, and starts the writer when it is not already running.
    ///
    /// <b>One write is out at a time, and the newest draft is the one that lands.</b> Two unary calls carry no
    /// ordering between them, so a burst, a port spinner held down, could finish out of order and store an older
    /// value than the screen shows.
    /// A write arriving while another is out replaces what is waiting rather than joining a queue: they are all
    /// the same settings, so the older ones have nothing left to say.
    ///
    /// Copied here for the reason every effect copies: the controls write the draft in place, so the live
    /// instance would let the next keystroke change the message mid-send.
    /// </summary>
    /// <returns>
    /// Answers once the run this write joined has drained, which a commit button waits on.
    /// A write superseded by a newer one answers with that newer one, they being the same settings.
    /// </returns>
    private Task Persist(Settings draft)
    {
        _pending = draft.Clone();
        _written ??= new TaskCompletionSource();
        var written = _written.Task;

        if (_persisting)
        {
            return written;
        }

        _persisting = true;
        _ = PersistAsync();
        return written;
    }

    /// <summary>
    /// Writes whatever is waiting, until nothing is, off the UI thread.
    /// Holds no state of its own: the answer goes back through the dispatcher to <see cref="Persisted"/>.
    /// </summary>
    private async Task PersistAsync()
    {
        try
        {
            while (_pending is not null)
            {
                var settings = _pending;
                _pending = null;

                try
                {
                    await _backend.SaveSettingsAsync(settings).ConfigureAwait(false);
                    _dispatch(() => Persisted("", settings));
                }
                catch (BackendUnavailableException e)
                {
                    _dispatch(() => Persisted(e.Message, stored: null));
                }
                catch (OperationCanceledException)
                {
                    // The call carries no token, so nothing here cancels it.
                    // A transport reporting one anyway keeps the last sentence rather than claiming a write landed.
                }
            }
        }
        finally
        {
            // Whatever ended the loop, the next write has to be able to start another run.
            // A flag left set by a task nobody awaits stops settings being stored for the rest of the session.
            //
            // Cleared before the waiters are answered: one of them may write again from its continuation,
            // and a run that answered while still claiming to run would take that write onto a queue nothing drains.
            _persisting = false;

            var written = _written;
            _written = null;
            written?.TrySetResult();
        }
    }

    /// <summary>
    /// Takes the answer to one write, on the UI loop.
    /// <b><see cref="Adopt"/> deliberately does not clear this.</b>
    /// A resolve is a read and can be answered while a write to the same backend is failing,
    /// so a successful read clearing the sentence would drop the news the reader needs:
    /// what the screen shows is not what is stored.
    /// </summary>
    /// <param name="stored">
    /// What the backend holds, null where the write did not land.
    /// The message that went over rather than the draft as it stands, so a keystroke made during the round trip
    /// leaves the settings reported as unstored.
    /// </param>
    private void Persisted(string reason, Settings? stored)
    {
        Unsaved = reason;

        if (stored is not null)
        {
            _stored = stored;
        }

        Announce();
    }

    /// <summary>
    /// Asks again after a failure.
    /// Two cases because the opening read has no draft in front of it:
    /// with settings in hand it is <see cref="Reask"/>, without them the opening read started over.
    /// </summary>
    public void Retry()
    {
        Unavailable = "";

        if (_draft is null)
        {
            Start(draft: null);
            Announce();
            return;
        }

        Reask();
    }

    /// <summary>
    /// <see cref="Retry"/>, awaited, which is what a button waits on.
    /// Either branch leaves the read it started in <see cref="Settled"/>, so this waits on the class's own notion
    /// of having caught up rather than on a task of its own.
    /// </summary>
    public async Task RetryAsync()
    {
        Retry();

        await Settled.ConfigureAwait(false);
    }

    /// <summary>
    /// Starts one read and supersedes whatever was out.
    /// The token asks the older call to stop, and the request number it stamps settles the race the token can
    /// lose.
    /// </summary>
    private void Start(Settings? draft)
    {
        _cancel?.Cancel();
        _cancel?.Dispose();
        _cancel = new CancellationTokenSource();
        _request++;
        _resolving = true;

        // The wait is for the run rather than for this call: an answer about a draft the reader has moved off
        // leaves the screen behind what they are holding, and the caller asked to know about that.
        _caught ??= new TaskCompletionSource();
        Settled = _caught.Task;

        _ = ResolveAsync(draft, _request, _cancel.Token);
    }

    /// <summary>
    /// Ends one answer's turn, on the UI loop.
    /// A draft the reader moved to while the answer was out is asked about here,
    /// which is the whole of the coalescing: writes during a round trip cost one round trip between them.
    /// </summary>
    private void Settle()
    {
        Sync();

        if (_resolving)
        {
            return;
        }

        // Cleared before the waiters are answered: one of them may write from its continuation,
        // and a run that answered while still claiming to run would leave that write on a source nothing completes.
        var caught = _caught;
        _caught = null;
        Settled = Task.CompletedTask;
        caught?.TrySetResult();
    }

    /// <summary>
    /// One read, off the UI thread.
    /// Writes nothing itself: the answer goes back through the dispatcher to <see cref="Adopt"/>, the only place
    /// the form and the draft are assigned.
    /// </summary>
    /// <param name="draft">
    /// Settings to resolve, null on the opening read, where the stored settings are the draft and fetching them
    /// is the hop in front of it.
    /// </param>
    private async Task ResolveAsync(Settings? draft, int request, CancellationToken cancellation)
    {
        try
        {
            // Only the opening read sees what the backend is holding.
            // Every read after it is asked about a draft,
            // and the answer describes that draft rather than the other side's settings.
            Settings? stored = null;
            if (draft is null)
            {
                stored = await _backend.SettingsAsync(cancellation).ConfigureAwait(false);
                draft = stored;
            }

            var form = await _backend.ResolveFormAsync(draft, cancellation).ConfigureAwait(false);

            _dispatch(() => Adopt(form, stored, request));
        }
        catch (OperationCanceledException)
        {
            // A newer draft superseded this one, and its answer is the one the screen wants.
            // This call ending is the point of having cancelled it.
        }
        catch (BackendUnavailableException e)
        {
            _dispatch(() => Fail(e.Message, request));
        }
    }

    /// <summary>
    /// Takes one answer, on the UI loop.
    /// The only write of <c>_form</c>, <c>_draft</c> and <c>_asked</c>.
    /// <b>The latest answer wins.</b>
    /// Cancellation is cooperative, so a call can hold its form by the time the token is set
    /// and land after a newer one.
    /// The request number makes that harmless rather than rare: an answer nothing is waiting for is dropped
    /// and the newer form stands.
    /// </summary>
    private void Adopt(Form form, Settings? stored, int request)
    {
        Assert.NotNull(form, "a resolve answers with the form it resolved");

        if (request != _request)
        {
            return;
        }

        _resolving = false;

        // Only the opening read carries them, and they are taken as they came rather than as the resolve
        // repaired them.
        // A repair nobody kept is not a value the backend runs on, and recording it as one hides the difference
        // _stored exists to hold.
        //
        // A draft that differs from the copy the backend last said it holds is uncommitted work, read here
        // against the previous answer because this is the one place that answer moves (Reread).
        var uncommitted = false;
        if (stored is not null)
        {
            uncommitted = _draft is not null && !_draft.Equals(_stored);
            _stored = stored;
        }

        _form = form;
        Unavailable = "";

        // Adopted whole rather than merged: where the backend walked a forbidden value to a legal one, merging
        // would be this class picking which half to keep.
        //
        // The draft is a copy and the form keeps its own, because the controls write the draft in place
        // and a write reaching into the form would edit the answer the screen is drawing.
        // The form's copy is what the next pass compares against, so a repaired draft counts as asked about
        // and settles here rather than costing a second round trip, which is the contract's idempotency.
        //
        // Taken only where the draft is still the one this answer describes.
        // A control moved while the answer was out holds a value newer than the repaired one, and adopting here
        // would drag the reader back to where the pointer was a round trip ago.
        // The form is drawn either way, being the newest answer there is, and Settle asks about the newer draft.
        //
        // A held thumb is the same case held open: the reader is on a value they have not settled on,
        // so the repair waits for the release (AdoptHeld).
        //
        // Uncommitted work stands for the same reason: an answer about the backend's own settings describes
        // what this window would open on, not what the reader has typed into it since.
        if (!_sweeping && !uncommitted && (_draft is null || _draft.Equals(_asked)))
        {
            _draft = form.Settings.Clone();
            _asked = form.Settings;
        }

        Settle();
        Announce();
    }

    /// <summary>
    /// Takes one refusal, on the UI loop.
    /// The form being drawn is kept and gains the sentence saying why there is no newer one:
    /// the last answer the backend gave is still the last answer it gave.
    /// <b><c>_asked</c> is left where it was, and that is load-bearing.</b>
    /// Cleared, the render pass this raises would find a draft the backend has not been asked about, resolve, fail
    /// and render again, hammering an absent socket for as long as the window is open.
    /// Asking again is <see cref="Retry"/>, which a reader runs when there is something new to expect.
    /// </summary>
    private void Fail(string reason, int request)
    {
        Assert.That(reason.Length > 0, "a read that could not be answered says why");

        if (request != _request)
        {
            return;
        }

        Unavailable = reason;
        Announce();
    }

    /// <summary>
    /// Reads the backend's settings again, because they moved for a reason this window did not cause:
    /// another window's write, or the backend's own, a landed Discord link being the one nothing else reports.
    ///
    /// The read with no draft in front of it, the shape the opening one has, so the answer is what the backend
    /// holds rather than what this window last handed over.
    /// <b>A draft equal to the copy the backend last said it holds becomes the new settings</b>, and
    /// uncommitted work stands: <c>_stored</c> moves under it either way, which is the difference the two of
    /// them exist to hold (<see cref="Adopt"/>).
    ///
    /// A window hears its own writes here too, the announcement naming no author.
    /// Idempotent, so that costs a read answering the settings this window just saved.
    /// </summary>
    public void Reread()
    {
        Start(draft: null);
        Announce();
    }

    /// <summary>
    /// Asks again for the draft on screen, because what the backend would answer has moved.
    /// The encoder probe landing raises it: forms resolved before it grey nothing for missing hardware,
    /// and the ones after it do.
    /// Clearing <c>_asked</c> is the whole of it: the draft is unchanged,
    /// so the round-trip guard would otherwise skip a read already answered against facts that have since changed.
    /// </summary>
    private void Reask()
    {
        _asked = null;
        Sync();
        Announce();
    }

    /// <summary>
    /// Asks again when the backend comes back, on the UI loop.
    /// <b>The transition is acted on, not the state.</b> The session reports the backend absent for as long as it
    /// is, so reading "reachable" would ask again on every event a healthy backend sends.
    /// The edge asks once, when there is something new to expect.
    /// No timer, for the reason <see cref="Fail"/> does not retry by itself: a timer would hammer an absent
    /// socket.
    /// The session's reconnect is not one either, being the connection the window already holds, saying it
    /// answered.
    /// The retry button is left for the failure nothing else reports, a read the backend served a refusal to.
    /// </summary>
    private void OnSessionChanged()
    {
        var absent = _session.Unavailable.Length > 0;
        var recovered = _backendWasAbsent && !absent;
        _backendWasAbsent = absent;

        if (recovered && Unavailable.Length > 0)
        {
            Retry();
        }

        // What publishes is the resolve's second input, and this is where news of it arrives.
        // Idempotent, so every event may ask: the guard skips the ones under which nothing moved.
        Sync();
    }

    private void Announce() => Changed?.Invoke();
}
