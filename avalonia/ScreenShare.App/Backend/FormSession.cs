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
    /// Which resolve is being waited for, counting up. An answer arriving with an older number
    /// belongs to a draft the reader has already moved off, and is dropped.
    /// </summary>
    private int _request;

    /// <summary>Cancels the resolve in flight when a newer draft supersedes it.</summary>
    private CancellationTokenSource? _cancel;

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
    /// Why the last read could not be answered, empty while the backend is answering. It is that
    /// side's own sentence, shown as it stands, and it is cleared by the next answer - which is
    /// what makes a recovered backend clear the notice a failed read left behind rather than
    /// leaving it under a form that is drawing again.
    /// </summary>
    public string Unavailable { get; private set; } = "";

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
            draft ??= await _backend.SettingsAsync(cancellation).ConfigureAwait(false);
            var form = await _backend.ResolveFormAsync(draft, cancellation).ConfigureAwait(false);

            _dispatch(() => Adopt(form, request));
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
    private void Adopt(Form form, int request)
    {
        Assert.NotNull(form, "a resolve answers with the form it resolved");

        if (request != _request)
        {
            return;
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
