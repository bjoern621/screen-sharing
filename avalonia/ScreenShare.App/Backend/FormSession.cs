using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Backend;

/// <summary>
/// The settings draft being edited, and the last form the backend resolved it to.
///
/// <b>It is the one owner of both, for the whole window.</b> Two screens edit settings - the
/// setup wizard configures what this machine sends, and the viewer configures how it receives -
/// and a draft each would be two copies of one fact: the wizard's commit persists the whole
/// message, so a viewer holding its own copy would have its watch settings overwritten by a
/// draft that never saw them. One owner read through by both is what removes that class of bug
/// rather than the instance (<c>docs/development-principles.md</c>, "Stateless").
///
/// It is the sibling of <see cref="Session"/> and the division between them is what each one
/// owns: the session holds the running state the backend reports, and this holds the settings
/// nobody has committed yet. Neither derives anything from the other.
///
/// <b>It decides nothing.</b> Which controls exist, which values they offer, which of those are
/// greyed and why, what the configuration costs and whether it can be published all come back
/// from <see cref="IBackend.ResolveFormAsync"/> already decided. This class puts a value in and
/// holds the answer that comes back (<c>docs/ipc-api.md</c>, "The rule").
///
/// <b>The resolve is a round trip, and that is what shapes the class.</b> The backend answers
/// over a socket, so a render pass cannot wait for it without freezing the window that is meant
/// to be drawing. The split the seam forces is the one the principles already ask for, stated
/// literally: the last form the backend answered with is <b>explicit state</b>, every reader
/// <b>reads it continuously</b> and never awaits anything, and a draft change is a write that
/// starts a resolve whose answer lands later and raises <see cref="Changed"/>. A window with no
/// form yet is an honest state rather than a gap.
///
/// Two properties make that safe to do on every keystroke. <see cref="Sync"/> is
/// <b>idempotent</b>: a call whose draft still equals the one the backend was last asked about
/// asks nothing, which is what lets a render pass reconcile unconditionally. And <b>the latest
/// answer wins</b>: each resolve carries a request number, the one before it is cancelled, and
/// an answer whose number is no longer the current one is dropped, so an older draft's form
/// cannot overwrite a newer draft's.
/// </summary>
public sealed class FormSession
{
    private readonly IBackend _backend;

    /// <summary>
    /// The running state, read through for one thing only: the edge on which the backend
    /// answers again after it could not be reached.
    /// </summary>
    private readonly Session _session;

    private readonly Action<Action> _dispatch;

    /// <summary>
    /// The last form the backend answered with, and null until the first answer lands. This is
    /// the state every reader draws from: it is written once per answer, by <see cref="Adopt"/>
    /// and by nothing else.
    /// </summary>
    private Form? _form;

    /// <summary>
    /// The settings being edited, and null until the stored settings arrive. Never read for
    /// meaning: a value goes in and the answer that comes back is what is drawn. It is written
    /// in place by <see cref="Write"/>, which is why nothing else ever holds this instance - the
    /// backend is handed a copy and the form keeps its own.
    /// </summary>
    private Settings? _draft;

    /// <summary>
    /// The draft the backend was last asked about: the copy handed to the resolve, replaced by
    /// the settings its answer carried. Nothing mutates it once it is set, so comparing the
    /// draft against it answers exactly one question - has anything moved since the backend was
    /// last asked - and that is the whole of the round-trip guard.
    /// </summary>
    private Settings? _asked;

    /// <summary>
    /// The settings the backend is holding, and null until the opening read answers.
    ///
    /// It is a different fact from the draft, and the difference is what a staged group means.
    /// Several effects are served off the backend's own settings rather than off anything a
    /// call hands over - a decode reads the render chain and both jitter buffers out of them
    /// (<c>docs/ipc-api.md</c>) - so this is what those effects will run on, where the draft is
    /// what the reader is looking at. They are equal until something is edited and equal again
    /// once it is kept.
    /// </summary>
    private Settings? _stored;

    /// <summary>
    /// Which resolve is being waited for, counting up. An answer arriving with an older number
    /// belongs to a draft the reader has already moved off, and is dropped.
    /// </summary>
    private int _request;

    /// <summary>Cancels the resolve in flight when a newer draft supersedes it.</summary>
    private CancellationTokenSource? _cancel;

    /// <summary>
    /// The settings waiting to be persisted, null when none are. Replaced rather than appended
    /// to: a newer draft says everything an older one did.
    /// </summary>
    private Settings? _pending;

    /// <summary>Whether a write is in flight, which is what keeps two from overlapping.</summary>
    private bool _persisting;

    /// <summary>
    /// Answers the caller that asked for a write, once the queue it joined has drained. One
    /// source serves every caller waiting on the same run, because they are all waiting for the
    /// same thing: the newest draft stored. Null while nothing is queued.
    /// </summary>
    private TaskCompletionSource? _written;

    /// <summary>
    /// Whether the session last reported the backend absent. It is held for one purpose: to tell
    /// the moment the backend came back from the state of it being there, so this asks again
    /// exactly once per recovery rather than on every event that follows one.
    /// </summary>
    private bool _backendWasAbsent;

    /// <param name="dispatch">
    /// Hands work to the UI loop. Injected rather than reached for, so this type stays free of a
    /// toolkit and a test can pass a synchronous dispatcher - the same arrangement
    /// <see cref="Session"/> uses, and for the same reason: the answer to a resolve arrives on
    /// whichever thread the transport completed on, and every reader of this writes bound
    /// properties from it.
    /// </param>
    public FormSession(IBackend backend, Session session, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a draft needs the backend that describes it");
        Assert.NotNull(session, "a draft watches the running state for a backend that came back");
        Assert.NotNull(dispatch, "a draft needs a UI loop to marshal an answer back to");

        _backend = backend;
        _session = session;
        _dispatch = dispatch;

        // News that the backend's answer has moved - the encoder probe landing is what raises it
        // today - and the response is to read again rather than to change anything held here.
        // Marshalled first: the signal arrives on whichever thread the transport completed on.
        _backend.Changed += () => _dispatch(Reask);

        // News that the running state moved, watched for one thing only: the backend answering
        // again after it could not be reached. Raised on the UI loop by the session itself, so
        // there is nothing to marshal here.
        _session.Changed += OnSessionChanged;

        // The one read with no draft in front of it: the stored settings are what this opens on
        // and there is nothing here to derive them from, so it is started once rather than
        // reconciled from a render pass the way every resolve after it is.
        Settled = Start(draft: null);
    }

    /// <summary>
    /// Raised on the UI loop after the form, the draft or the reason there is no newer form has
    /// moved. It carries nothing: the form is whole and is read through, so the news that
    /// something changed and the thing it changed to are two different facts and only the first
    /// belongs on a signal.
    /// </summary>
    public event Action? Changed;

    /// <summary>
    /// The last form the backend answered with, null until the first answer lands. Read through
    /// on every render pass and never copied: a reader that cached it would go on drawing an
    /// older answer than the one the draft resolves to now.
    /// </summary>
    public Form? Form => _form;

    /// <summary>
    /// The settings being edited, null until the stored settings arrive. Read by a commit, which
    /// takes a copy of it: the controls write this instance in place, so handing the live one to
    /// an effect would let a keystroke change the settings while they are being sent.
    /// </summary>
    public Settings? Draft => _draft;

    /// <summary>
    /// The settings the backend is holding, null until the opening read answers.
    ///
    /// Read by whatever has to name a value the backend will act on rather than one the reader
    /// is looking at. The leg a decode is opened on is the case that exists: the backend reads
    /// every other knob of that decode out of these same settings, so opening on the draft would
    /// run half a panel's worth of choices and hold the other half back.
    /// </summary>
    public Settings? Stored => _stored;

    /// <summary>
    /// Why the last read could not be answered, empty while the backend is answering. It is that
    /// side's own sentence, shown as it stands, and it is cleared by the next answer - which is
    /// what makes a recovered backend clear the notice a failed read left behind rather than
    /// leaving it under a form that is drawing again.
    /// </summary>
    public string Unavailable { get; private set; } = "";

    /// <summary>
    /// Why the last write could not be stored, empty while they are being stored. It is that
    /// side's own sentence, shown as it stands.
    ///
    /// It is separate from <see cref="Unavailable"/> because the two are different news and one
    /// must not stand in for the other. A read that cannot be answered leaves the screen showing
    /// an older answer, and a publish has nothing to go on. A write that cannot be stored leaves
    /// the screen showing exactly what the reader typed while the backend goes on running on the
    /// value before it - so the setting is worth naming and the publish is not worth blocking.
    /// </summary>
    public string Unsaved { get; private set; } = "";

    /// <summary>
    /// The read in flight, and an already-completed task when none is. It is the seam's timing
    /// made observable, for the one caller that legitimately needs it: something that has to know
    /// the screen has caught up with the draft rather than merely having been asked to. A test
    /// waits on it instead of sleeping; nothing in a render path touches it. It never faults on a
    /// cancellation, because a cancelled resolve is one this class asked for.
    /// </summary>
    public Task Settled { get; private set; } = Task.CompletedTask;

    /// <summary>
    /// Converges the backend onto the draft: asks for the form this draft resolves to, unless it
    /// has already been asked for it.
    ///
    /// <b>Idempotent, and that is what makes it safe on a render pass.</b> The contract states
    /// the resolve is side-effect free and answers the same form for the same draft
    /// (<c>docs/ipc-api.md</c>), so a draft that still equals the one last handed over has
    /// nothing to learn from a second round trip - whether that first answer has landed or is
    /// still in flight. Rendering twice therefore costs one call, not two, and rendering a
    /// hundred times costs the same one.
    /// </summary>
    public void Sync()
    {
        // Before the stored settings arrive there is no draft to describe, and the read that
        // fetches them is already in flight from the constructor.
        if (_draft is null || _draft.Equals(_asked))
        {
            return;
        }

        // A copy, because the controls write the draft in place: handing the live instance over
        // would let the next keystroke change the message while it is being sent, and would leave
        // the answer describing settings nobody ever asked about.
        var draft = _draft.Clone();
        _asked = draft;
        Settled = Start(draft);
    }

    /// <summary>
    /// One field write, arriving from whichever control the reader moved. The draft is changed
    /// and the whole thing re-resolved: which other controls that frees or greys is the backend's
    /// answer, and asking for it is cheap by contract.
    /// </summary>
    public void Write(string key, FieldValue value)
    {
        // A control the reader can move was drawn from a form, and a form was resolved from a
        // draft, so a write arriving without one means a field was rendered from nothing.
        var draft = Assert.NotNull(_draft, "a control the reader moved was drawn from a draft");
        Assert.That(key.Length > 0, "a write names the settings field it changes");

        SettingsDraft.Write(draft, key, value);
        Sync();

        // A field of an applied group is the setting itself rather than a proposal a commit
        // turns into one, so the write is persisted as it is made (<c>form.proto</c>,
        // FieldGroup.applied). Which groups those are is the backend's answer, read off the
        // form; this class asks the question and does not answer it.
        if (Applies(key))
        {
            _ = Persist(draft);
        }

        Announce();
    }

    /// <summary>
    /// Keeps the draft as it stands, and answers once the write has landed.
    ///
    /// It is what a staged group's commit runs: nothing about those fields reaches the backend
    /// as they are edited, so a screen drawing one needs a way to say "these, now". The write is
    /// the whole settings message either way, so this is the same effect an applied field's
    /// keystroke starts and it goes down the same queue - a commit racing an applied write would
    /// be two unary calls with no ordering between them, and the older snapshot landing last is
    /// exactly what <see cref="Persist"/> exists to prevent.
    ///
    /// <b>Safe to run twice.</b> It names a state - these are the stored settings - so a second
    /// run with nothing changed asks for a state that already holds
    /// (<c>docs/development-principles.md</c>, "Effects across a process boundary"). Whether it
    /// landed is <see cref="Unsaved"/>; this answers when the attempt is over, either way.
    /// </summary>
    public Task SaveAsync()
    {
        // A commit was offered beside a form, and a form was resolved from a draft.
        var draft = Assert.NotNull(_draft, "a save that was offered was drawn from a draft");

        return Persist(draft);
    }

    /// <summary>
    /// Replaces the way of publishing whole, which is what applying a preset is. The draft's
    /// other two groups are untouched: a preset is a <c>PublishSettings</c> and nothing else, so
    /// where the relay is and how this machine watches are not its to say
    /// (<c>docs/presets.md</c>).
    ///
    /// <b>An assignment and not a merge.</b> The settings travel whole for that reason
    /// (<c>settings.proto</c>, <c>Preset</c>): merged, what a preset produced would depend on
    /// what the form happened to hold first, and the same preset would mean different pictures
    /// on two machines. What comes back from the resolve is the repaired version of exactly
    /// what was assigned, which is the same adoption every other write ends in.
    ///
    /// This is the one write that names a settings group. Every other one addresses a field by
    /// the key the form gave it (<see cref="Write"/>), and there is no key for a whole group -
    /// nor should there be, since no control writes one. What makes naming it honest here is
    /// that the contract names it too: a preset is defined as that group.
    /// </summary>
    public void WritePublish(PublishSettings publish)
    {
        Assert.NotNull(publish, "applying a preset needs the way of publishing it saved");

        // A preset can only be applied to a form the reader is looking at, and a form was
        // resolved from a draft.
        var draft = Assert.NotNull(_draft, "a preset the reader applied was offered beside a draft");

        // A copy, because the store's message is held for as long as the list on screen is: an
        // assignment of the instance itself would let the next keystroke edit the preset.
        draft.Publish = publish.Clone();
        Sync();

        // The same question a field write asks, asked about every field this one moved: are
        // these settings themselves, or a proposal a commit turns into settings? Publish
        // settings are staged, so this stores nothing and a preset the reader is trying out is
        // not what the next stream starts on. It is read off the form rather than stated here,
        // so a group that becomes applied is persisted by this write too.
        if (AppliesToGroup(SettingsDraft.PublishGroup))
        {
            _ = Persist(draft);
        }

        Announce();
    }

    /// <summary>
    /// Puts one group of settings back to what a fresh installation holds.
    ///
    /// <b>The values are the form's, stated per field</b> (<c>form.proto</c>,
    /// <c>Field.default_value</c>). This side holds no defaults of its own and could not: what a
    /// setting starts as is the same fact as which values it may take, and both are the
    /// backend's (<c>docs/ipc-api.md</c>, "The rule"). So a group that gains a field is a field
    /// this puts back with nothing here to edit.
    ///
    /// <b>One write, not one per field.</b> Every field of the group goes into the draft before
    /// anything is asked or stored, so the resolve sees the whole reset and an applied group is
    /// persisted once rather than once per port. Both follow from what the reset is: a single
    /// change of mind about a group, rather than a burst of the writes a reader could have made
    /// by hand.
    /// </summary>
    public void Reset(string groupKey)
    {
        Assert.That(groupKey.Length > 0, "a reset names the group it puts back");

        // Both are what the offer was drawn from: a heading the reader pressed came from a form,
        // and a form was resolved from a draft.
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

    /// <summary>The group under this key in the last form, or null where there is none.</summary>
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
    /// Whether a write to this field is the setting itself, which the form states per group
    /// (<c>form.proto</c>, FieldGroup.applied).
    ///
    /// False for a key no drawn group carries, and before the first form lands. Both are the
    /// same answer for the same reason: what a field means arrives from the backend, and a
    /// write it has said nothing about is one this class holds rather than one it persists on a
    /// guess.
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
    /// Whether a write to any field of one settings group is the setting itself.
    ///
    /// It is <see cref="Applies"/> asked about a group, which is what a whole-group write needs.
    /// The two questions are not the same one: the form groups the screen by what the reader is
    /// deciding and a key by which message holds the value, so one settings group's fields reach
    /// the screen spread across several form groups (<c>internal/form/keys.go</c>). Any of them
    /// being applied makes the write a setting, because that is the field the backend would
    /// otherwise never be handed.
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
    /// Queues the draft to be persisted, and starts the writer when it is not already running.
    ///
    /// <b>One write is in flight at a time, and the newest draft is the one that lands.</b> Two
    /// unary calls carry no ordering between them, so a burst - a port spinner held down - could
    /// otherwise finish out of order and leave an older value stored than the one on screen. A
    /// write that arrives while another is in flight replaces what is waiting rather than
    /// joining a queue: they are all the same settings, so the older ones have nothing left to
    /// say.
    ///
    /// The copy is taken here for the reason every other effect takes one: the controls write
    /// the draft in place, so handing the live instance over would let the next keystroke change
    /// the message while it is being sent.
    /// </summary>
    /// <returns>
    /// Answers when the run this write joined has drained, for a caller that has something to do
    /// once it has - a commit button waits on it. A write superseded by a newer one answers with
    /// that newer one, because they are the same settings and the newest is the one that says
    /// anything.
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
    /// Writes whatever is waiting, until nothing is, off the UI thread. It writes no state of
    /// its own: the sentence goes back through the dispatcher to <see cref="Persisted"/>.
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
                    // Nothing cancels this call, since it carries no token. A transport that
                    // reports one anyway leaves the last sentence standing rather than claiming
                    // a write landed.
                }
            }
        }
        finally
        {
            // Whatever ended the loop, including something nothing here expected, the next write
            // has to be able to start another one. The alternative is a flag left set by a task
            // nobody is awaiting, and settings that silently stop being stored for the rest of
            // the session.
            //
            // Cleared before the waiters are answered, because one of them may write again from
            // the continuation: a run that answered while it still claimed to be running would
            // take that write onto a queue nothing is left to drain.
            _persisting = false;

            var written = _written;
            _written = null;
            written?.TrySetResult();
        }
    }

    /// <summary>
    /// Takes the answer to one write, on the UI loop.
    ///
    /// <b><see cref="Adopt"/> does not clear this and that is deliberate.</b> A resolve is a
    /// read: it can be answered while a write to the same backend is failing, and letting a
    /// successful read clear the sentence would drop the one piece of news the reader needs -
    /// that what the screen shows is not what is stored.
    /// </summary>
    /// <param name="stored">
    /// What the backend now holds, and null where the write did not land. It is the message that
    /// went over rather than the draft as it stands, so a keystroke made during the round trip
    /// leaves the settings correctly reported as not yet stored.
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
    /// Asks again after a failure. Two cases, because the first read has no draft in front of it:
    /// with settings in hand this is <see cref="Reask"/>, and without them it is the opening read
    /// started over.
    /// </summary>
    public void Retry()
    {
        Unavailable = "";

        if (_draft is null)
        {
            Settled = Start(draft: null);
            Announce();
            return;
        }

        Reask();
    }

    /// <summary>
    /// The same retry, awaited, which is what a button waits on. The read it starts is
    /// <see cref="Settled"/> either way - the first branch assigns it and the second reaches it
    /// through <see cref="Reask"/> - so this waits on this class's own notion of having caught up
    /// rather than on a task of its own.
    /// </summary>
    public async Task RetryAsync()
    {
        Retry();

        await Settled.ConfigureAwait(false);
    }

    /// <summary>
    /// Starts one read and supersedes whatever was in flight. The token asks the older call to
    /// stop; the request number it stamps is what settles the race the token can lose.
    /// </summary>
    private Task Start(Settings? draft)
    {
        _cancel?.Cancel();
        _cancel?.Dispose();
        _cancel = new CancellationTokenSource();
        _request++;

        return ResolveAsync(draft, _request, _cancel.Token);
    }

    /// <summary>
    /// One read, off the UI thread. It writes nothing itself: the answer goes back through the
    /// dispatcher to <see cref="Adopt"/>, which is the only place the form and the draft are
    /// assigned.
    /// </summary>
    /// <param name="draft">
    /// The draft to resolve, or null on the first read, where the stored settings are the draft
    /// and fetching them is the hop in front of it.
    /// </param>
    private async Task ResolveAsync(Settings? draft, int request, CancellationToken cancellation)
    {
        try
        {
            // The opening read is the one that sees what the backend is holding, and the only
            // one: every read after it is asked about a draft, and what the answer describes is
            // that draft rather than the other side's settings.
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
            // A newer draft superseded this one. Its answer is the one the screen wants, and this
            // call ending is the point of having cancelled it.
        }
        catch (BackendUnavailableException e)
        {
            _dispatch(() => Fail(e.Message, request));
        }
    }

    /// <summary>
    /// Takes one answer, on the UI loop. The only write of <c>_form</c>, <c>_draft</c> and
    /// <c>_asked</c>.
    ///
    /// <b>The latest answer wins.</b> Cancellation is cooperative: a call can already have
    /// produced its form by the time the token is set, so an answer to a draft the reader has
    /// moved off can still arrive, and arrive after a newer one. The request number is what makes
    /// that harmless rather than rare - an answer that is not the one being waited for is
    /// dropped, and the newer form stands.
    /// </summary>
    private void Adopt(Form form, Settings? stored, int request)
    {
        Assert.NotNull(form, "a resolve answers with the form it resolved");

        if (request != _request)
        {
            return;
        }

        // Only the opening read carries them, and they are taken as they came rather than as the
        // resolve repaired them: a repair the reader has not kept is not a value the backend is
        // running on, and recording it as one would hide the very difference this holds.
        if (stored is not null)
        {
            _stored = stored;
        }

        _form = form;
        Unavailable = "";

        // Adopted wholesale rather than merged: where the backend walked a forbidden value to a
        // legal one, the merge would be this class deciding which half to keep.
        //
        // The draft is a copy of it and the form keeps its own, because the controls write the
        // draft in place and a write reaching into the form would edit the answer the screen is
        // drawing. The form's copy is what the next pass compares against, so a repaired draft
        // counts as asked about and settles here rather than costing a second round trip - which
        // the contract's idempotency is exactly the promise of.
        _draft = form.Settings.Clone();
        _asked = form.Settings;

        Announce();
    }

    /// <summary>
    /// Takes one refusal, on the UI loop. Whatever form was being drawn is kept and gains the
    /// sentence saying why there is no newer one, which is the honest pair: the last answer the
    /// backend gave is still the last answer it gave.
    ///
    /// <b>It leaves <c>_asked</c> where it was, and that is load-bearing.</b> Clearing it would
    /// mean the render pass this triggers finds a draft the backend has not been asked about,
    /// starts a resolve, fails, renders again - a loop that would hammer an absent socket for as
    /// long as the window is open. Asking again is <see cref="Retry"/>, which a reader runs when
    /// they have something new to expect.
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
    /// Asks the backend again for the draft on screen, because what it would answer has moved.
    /// The encoder probe landing is what raises that today: the forms resolved before it grey
    /// nothing for missing hardware, and the ones after it do.
    ///
    /// Clearing <c>_asked</c> is the whole of it. The draft is unchanged, so the round-trip guard
    /// would otherwise skip the read as one already answered - which it is, against facts that
    /// have since changed.
    /// </summary>
    private void Reask()
    {
        _asked = null;
        Sync();
        Announce();
    }

    /// <summary>
    /// Asks again when the backend comes back, on the UI loop.
    ///
    /// <b>It is the transition that is acted on, not the state.</b> The session reports the
    /// backend absent for as long as it is, so reacting to "reachable" would ask again on every
    /// event a healthy backend sends; reacting to the edge asks once, when there is something new
    /// to expect.
    ///
    /// That is the same reason <see cref="Fail"/> gives for not retrying by itself, arrived at
    /// from the other side. A timer here would hammer an absent socket, and the session's
    /// reconnect is not a timer of this class's - it is the one connection the window already
    /// holds, saying it answered. So a retry button stays for the failure nothing else notices -
    /// a read the backend served a refusal to - and stops being the only way back from the
    /// failure something does.
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
    }

    private void Announce() => Changed?.Invoke();
}
