using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Setup.AdvancedDrawer.ViewModel;
using ScreenShare.App.Features.Setup.CostRail.ViewModel;
using ScreenShare.App.Features.Setup.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.QualityStep.ViewModel;
using ScreenShare.App.Features.Setup.ReviewStep.ViewModel;
using ScreenShare.App.Features.Setup.StepStrip.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.ViewModel;

/// <summary>
/// The setup flow: which step is showing, the draft being edited, and the strip that is at
/// once its navigation, its progress and its summary.
///
/// <b>It owns the draft and nothing else.</b> Which steps exist, which controls each one
/// draws, what they are called, which values they offer, which of those are greyed and why,
/// what the configuration is predicted to cost and the line each step's chip carries all come
/// back from <see cref="IBackend.ResolveFormAsync"/> already decided. This class routes a group
/// to a step and a write to the backend, and evaluates no rule of its own (docs/ipc-api.md,
/// "The rule").
///
/// <b>The steps are the form's groups.</b> They used to be a table here, and three of the
/// seven rows named group keys the backend does not answer with - so three steps of the wizard
/// drew an empty column, and the four groups the table did not name were unreachable. Deriving
/// them removes the class of bug rather than the instance: a group added to the contract is a
/// step that appears and works with nothing here to edit (<see cref="SetupSteps"/>).
///
/// <b>Inputs</b> are <see cref="CurrentStep"/> and the field writes that arrive through
/// <see cref="Write"/>. Both end in <see cref="Apply"/>.
///
/// <b>Outputs</b> are written by <see cref="Apply"/> on every pass, including the branches
/// that turn a form off.
///
/// <b>The resolve is a round trip, and that is what shapes the rest of this class.</b> The
/// backend answers over a socket, so a render pass cannot wait for it without freezing the
/// window that is meant to be drawing. The split the seam forces is therefore the one
/// docs/development-principles.md already asks for, stated literally: the last form the
/// backend answered with is <b>explicit state</b> held in <c>_form</c>, the render pass
/// <b>reads it continuously</b> and never awaits anything, and a draft change is a write
/// that starts a resolve whose answer lands on a later pass and re-renders. A flow with no
/// form yet is an honest state rather than a gap - the strip is empty, every group renders
/// its unresolved branch, and the sentence saying why sits above the column.
///
/// Two properties make that safe to do on every keystroke. It is <b>idempotent</b>: a pass
/// whose draft still equals the one the backend was last asked about asks nothing, which is
/// what lets <see cref="Apply"/> reconcile unconditionally the way every other render
/// function here does. And <b>the latest answer wins</b>: each resolve carries a request
/// number, the one before it is cancelled, and an answer whose number is no longer the
/// current one is dropped, so an older draft's form cannot overwrite a newer draft's.
/// </summary>
public sealed class SetupViewModel : Observable
{
    /// <summary>
    /// Which form the main column draws. Three, and the split is layout rather than substance:
    /// every one of them renders a group of the resolved form.
    /// </summary>
    private enum StepContent
    {
        /// <summary>A group of the resolved form, drawn by the one generic renderer.</summary>
        Fields,

        /// <summary>
        /// The same thing in a layout of its own: cards for the choice whose options carry a
        /// paragraph each, a banded track for the scale with named ends, and a row of
        /// dropdowns for what is read back from the source.
        /// </summary>
        Quality,

        /// <summary>The review, which asks whether anything blocks the publish rather than editing a setting.</summary>
        Review,
    }

    private readonly IBackend _backend;

    /// <summary>
    /// The running state, owned by the window and read through here on every pass. The flow
    /// holds no copy of it: what is publishing and what the relay answered are two of the four
    /// things the commit turns on, and a second reading of either is a second opinion about it.
    /// </summary>
    private readonly Session _session;

    private readonly Action<Action> _dispatch;

    /// <summary>
    /// One select command per step key, made once and reused. Reusing the instance is what
    /// lets a chip row be a record: two passes over the same step then compare equal, rather
    /// than merely looking alike, and the strip is left alone.
    /// </summary>
    private readonly Dictionary<string, DelegateCommand> _select = [];

    /// <summary>
    /// One renderer per group key, kept across passes and made on demand. The form-driven
    /// steps differ in nothing this layer can see, so they are instances of one component
    /// rather than one component each.
    /// </summary>
    private readonly Dictionary<string, FieldGroupViewModel> _groups = [];

    // --- What the screen is drawn from ---------------------------------------------

    /// <summary>
    /// The last form the backend answered with, and null until the first answer lands. This
    /// is the state the render pass reads: it is written once per answer, by
    /// <see cref="Adopt"/> and by nothing else.
    /// </summary>
    private Form? _form;

    /// <summary>
    /// The settings being edited, and null until the stored settings arrive. Never read for
    /// meaning: the flow puts a value in and draws the answer that comes back. It is written
    /// in place by <see cref="Write"/>, which is why nothing else ever holds this instance -
    /// the backend is handed a copy and the form keeps its own.
    /// </summary>
    private Settings? _draft;

    /// <summary>
    /// The draft the backend was last asked about: the copy handed to the resolve, replaced
    /// by the settings its answer carried. Nothing mutates it once it is set, so comparing
    /// the draft against it answers exactly one question - has anything moved since the
    /// backend was last asked - and that is the whole of the round-trip guard.
    /// </summary>
    private Settings? _asked;

    /// <summary>
    /// Which resolve the flow is waiting for, counting up. An answer arriving with an older
    /// number belongs to a draft the reader has already moved off, and is dropped.
    /// </summary>
    private int _request;

    /// <summary>Cancels the resolve in flight when a newer draft supersedes it.</summary>
    private CancellationTokenSource? _cancel;

    /// <summary>
    /// Why the last read could not be answered, empty while the backend is answering. Written
    /// by <see cref="Adopt"/> and <see cref="Fail"/> only, and read through by the render pass
    /// the same way the form is - which is what makes a recovered backend clear the notice a
    /// failed read left behind, rather than leaving it under a form that is drawing again.
    /// </summary>
    private string _unreachable = "";

    /// <summary>
    /// Whether an uplink measurement is running. It uploads a real payload and takes seconds,
    /// so the button says so rather than going quietly inert, and a second press while one is
    /// in flight is refused rather than queued.
    /// </summary>
    private bool _measuring;

    /// <summary>
    /// Whether a start this flow asked for is still in flight. It locks the commit for exactly
    /// as long as the round trip lasts, so a second press cannot ask for a second stream while
    /// the backend is still deciding about the first.
    /// </summary>
    private bool _starting;

    /// <summary>
    /// Why the backend refused the last start, empty otherwise. It is that side's own sentence
    /// and is shown as it stands: a refusal is prose written for a person
    /// (<c>docs/ipc-api.md</c>, "Errors").
    /// </summary>
    private string _refusal = "";

    /// <summary>
    /// The steps the last pass rendered. Held because moving through them is an input rather
    /// than a render - Back and Continue need the order, and the order is the form's.
    /// </summary>
    private IReadOnlyList<SetupStepRow> _steps = [];

    /// <param name="dispatch">
    /// Hands work to the UI loop. Injected rather than reached for, so this type stays free
    /// of a toolkit and a test can pass a synchronous dispatcher - the same arrangement
    /// <c>RelayPoller</c> uses, and for the same reason: the answer to a resolve arrives on
    /// whichever thread the transport completed on, and every property below is read by a
    /// binding that only tolerates being written from one.
    /// </param>
    public SetupViewModel(IBackend backend, Session session, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a setup flow needs the backend that describes it");
        Assert.NotNull(session, "a setup flow reads the running state the commit turns on");
        Assert.NotNull(dispatch, "a setup flow needs a UI loop to marshal an answer back to");

        _backend = backend;
        _session = session;
        _dispatch = dispatch;

        // Everything the render function reads is built before anything can call it, so a
        // step moved from a child's constructor would still find a complete view model.
        Steps = [];
        BackCommand = new DelegateCommand(Back, () => CanGoBack);
        ContinueCommand = new DelegateCommand(Continue, () => CanContinue);
        RetryCommand = new DelegateCommand(Retry, () => IsUnavailable);

        // News that the backend's answer has moved - the encoder probe landing is what raises
        // it today - and the flow's response is to read again rather than to change anything
        // it holds. Marshalled first: the signal arrives on whichever thread the transport
        // completed on, and everything past this line writes bound properties.
        _backend.Changed += () => _dispatch(Reask);

        // The one group with a layout of its own, made eagerly because two children hold it.
        // Which controls it draws is still the form's answer - the step picks its fields out
        // of the group by key, and picking a place for a field is placement.
        Quality = new QualityStepViewModel(Group(QualityLayout.GroupKey));

        // The same group, and the other half of it: the drawer draws the fields the step's
        // layout places nowhere, so between them every control the backend offered is
        // reachable exactly once (Model/QualityLayout.cs).
        Advanced = new AdvancedDrawerViewModel(Group(QualityLayout.GroupKey));

        Rail = new CostRailViewModel(Measure, () => !_measuring && _draft is not null);
        Review = new ReviewStepViewModel(SelectCommandOf, Back, GoLive);

        // Rendered before anything is asked for, so the window has a complete view model to
        // paint whether or not the backend is reachable, and the first form is a later pass
        // rather than a precondition of the first one.
        Apply();

        // The one read with no draft in front of it: the stored settings are what the flow
        // opens on and there is nothing here to derive them from, so it is started once
        // rather than reconciled from a render pass the way every resolve after it is.
        Settled = Start(draft: null);
    }

    /// <summary>
    /// Raised once the backend has accepted a start this flow asked for. It is news that the
    /// commit went through and carries nothing, in the way every other signal here does: what
    /// the stream became arrives on the event stream, and the window reads it there.
    ///
    /// Whoever hosts this flow owns what happens next, because what happens next is a change of
    /// destination and the destination is the window's state rather than this flow's.
    /// </summary>
    public event Action? WentLive;

    // --- Input --------------------------------------------------------------------

    private string _currentStep = "";

    /// <summary>
    /// The step showing, named by the form group it draws. The strip is non-linear on
    /// purpose: a returning reader clicks straight to the encode step and goes live, and
    /// nothing requires walking the steps in order.
    ///
    /// Empty until the first form lands, and a key the newest form no longer carries is not
    /// an error - the render pass falls back to the first step rather than drawing nothing.
    /// </summary>
    public string CurrentStep
    {
        get => _currentStep;
        set
        {
            if (Set(ref _currentStep, value))
            {
                Apply();
            }
        }
    }

    // --- Outputs ------------------------------------------------------------------

    private bool _showsFields;
    private bool _showsQuality;
    private bool _showsReview;
    private bool _isRailVisible;
    private bool _canGoBack;
    private bool _canContinue;
    private bool _isPublishable;
    private string _continueLabel = "";
    private string _headline = "";
    private string _commandError = "";
    private bool _hasCommandError;
    private string _unavailable = "";
    private bool _isUnavailable;
    private FieldGroupViewModel? _currentGroup;

    public QualityStepViewModel Quality { get; }

    public AdvancedDrawerViewModel Advanced { get; }

    public CostRailViewModel Rail { get; }

    public ReviewStepViewModel Review { get; }

    /// <summary>
    /// Whether the reader asked to be taken to the broadcast screen when the stream starts. It
    /// is the review's own switch, read by the window after <see cref="WentLive"/>: which
    /// destination is showing is the window's state, and this flow neither holds nor writes it.
    /// </summary>
    public bool OpensBroadcastWindow => Review.OpenBroadcastWindow;

    public ObservableCollection<StepChipViewModel> Steps { get; }

    public DelegateCommand BackCommand { get; }

    public DelegateCommand ContinueCommand { get; }

    /// <summary>
    /// Asks again after the backend could not answer. It is a command rather than a timer
    /// because a retry loop would hammer an absent socket for as long as the window is open,
    /// and because the reader is the one who knows they have just started the backend.
    /// </summary>
    public DelegateCommand RetryCommand { get; }

    /// <summary>
    /// The read in flight, and an already-completed task when none is. It is the seam's
    /// timing made observable, for the one caller that legitimately needs it: something that
    /// has to know the screen has caught up with the draft rather than merely having been
    /// asked to. A test waits on it instead of sleeping; nothing in the render path touches
    /// it. It never faults on a cancellation, because a cancelled resolve is one this flow
    /// asked for.
    /// </summary>
    public Task Settled { get; private set; } = Task.CompletedTask;

    /// <summary>The renderer for the step showing, null on a step that draws something else.</summary>
    public FieldGroupViewModel? CurrentGroup { get => _currentGroup; private set => Set(ref _currentGroup, value); }

    public bool ShowsFields { get => _showsFields; private set => Set(ref _showsFields, value); }

    public bool ShowsQuality { get => _showsQuality; private set => Set(ref _showsQuality, value); }

    public bool ShowsReview { get => _showsReview; private set => Set(ref _showsReview, value); }

    /// <summary>
    /// The rail steps aside on the review, which carries the same list beside its own commit.
    /// Two copies of it on one screen would be two things to read and one of them redundant.
    /// </summary>
    public bool IsRailVisible { get => _isRailVisible; private set => Set(ref _isRailVisible, value); }

    public bool CanGoBack { get => _canGoBack; private set => Set(ref _canGoBack, value); }

    public bool CanContinue { get => _canContinue; private set => Set(ref _canContinue, value); }

    /// <summary>
    /// Whether the settings can be published as they stand. Stated by the form rather than
    /// ranked here, so the button and the refusal are one answer. False while no form has
    /// arrived, which is the honest reading of settings nothing has vouched for yet.
    /// </summary>
    public bool IsPublishable { get => _isPublishable; private set => Set(ref _isPublishable, value); }

    /// <summary>The whole configuration in one line, composed by the backend.</summary>
    public string Headline { get => _headline; private set => Set(ref _headline, value); }

    /// <summary>Why no command could be rendered for these settings, empty when one was.</summary>
    public string CommandError { get => _commandError; private set => Set(ref _commandError, value); }

    public bool HasCommandError { get => _hasCommandError; private set => Set(ref _hasCommandError, value); }

    /// <summary>
    /// Why the backend could not describe the screen, empty while it can. It is the backend's
    /// own sentence, shown as it stands: a shell with nothing to talk to says so rather than
    /// drawing a form it made up (docs/ipc-api.md, "What each side owes").
    /// </summary>
    public string Unavailable { get => _unavailable; private set => Set(ref _unavailable, value); }

    public bool IsUnavailable { get => _isUnavailable; private set => Set(ref _isUnavailable, value); }

    /// <summary>Names the next step rather than saying "Next", so the button says where it goes.</summary>
    public string ContinueLabel { get => _continueLabel; private set => Set(ref _continueLabel, value); }

    /// <summary>
    /// The one render function, and it is synchronous. It draws the last form the backend
    /// answered with and never waits for a newer one: asking is <see cref="Resolve"/>, whose
    /// answer arrives on a later pass.
    ///
    /// Safe to run twice: the resolve it reconciles is skipped when the draft has not moved,
    /// and every row it produces compares equal to the last one, so an unchanged pass fires
    /// no binding.
    /// </summary>
    public void Apply()
    {
        // Reconciled from the render pass rather than performed by it, which is the same
        // arrangement MainViewModel has with its poller: the pass states what it wants and
        // the converge decides whether anything has to be asked
        // (docs/development-principles.md, "Idempotency").
        Resolve();

        // Read through once, so every output below is derived from one form rather than from
        // whatever the field held at the moment each of them was written.
        var form = _form;

        // A renderer per group the form carries, then a pass over every renderer this flow
        // holds - including the ones the newest form dropped, which is what makes them clear
        // rather than go on showing what an older form said.
        if (form is not null)
        {
            foreach (var group in form.Groups)
            {
                Group(group.Key);
            }
        }

        foreach (var key in _groups.Keys.ToList())
        {
            _groups[key].Apply(form is null ? null : GroupOf(form, key), _session.Words, form?.Settings);
        }

        IReadOnlyList<FieldGroup> groups = form is null ? [] : form.Groups;
        IReadOnlyList<Diagnostic> diagnostics = form is null ? [] : form.Diagnostics;

        _steps = SetupSteps.For(groups);
        var current = Standing(_steps);

        // The rail before the strip: the terminal chip repeats the rail's own summary, so it
        // has to be the summary of the list the rail is about to draw rather than of the one
        // it drew last pass.
        var checks = PreflightChecks.Of(diagnostics, AnchorIn(_steps, form));
        Rail.Apply(form?.Summary?.Estimate, Uplink(), checks, _measuring);

        IsPublishable = form?.Publishable ?? false;

        // Composed on every pass out of four states nothing here owns: the form's own verdict on
        // the settings, whether the backend answered at all, whether a stream is already in
        // force, and whether the relay is there to publish to. Read through rather than cached,
        // so a relay that came back unlocks the button on the next pass without anything having
        // to remember that it was locked.
        var gate = PublishGate.Of(IsPublishable, _unreachable, _session.Publish, _session.Relay, _starting);
        Review.Apply(gate, _draft?.Publish?.Name ?? "", _refusal, Summaries(form), checks);

        Reconcile.Onto(Steps, StepChips.For(_steps, current, ValueOf, SelectCommandOf));

        var content = ContentOf(current);
        ShowsFields = content == StepContent.Fields;
        ShowsQuality = content == StepContent.Quality;
        ShowsReview = content == StepContent.Review;
        IsRailVisible = content != StepContent.Review;
        CurrentGroup = ShowsFields && current.Length > 0 ? Group(current) : null;

        // The one-line shorthand for the whole configuration. Composed here out of the draft
        // the form carried, for the reason each group's is: it picks a separator, an
        // abbreviation and a length, none of which is visible from the backend.
        Headline = _session.Words.Headline(form?.Settings?.Publish);
        CommandError = form?.Summary?.CommandError ?? "";
        HasCommandError = CommandError.Length > 0;
        Unavailable = _unreachable;
        IsUnavailable = Unavailable.Length > 0;

        var next = SetupSteps.After(_steps, current);
        CanContinue = next is not null;
        ContinueLabel = next is null ? "" : $"Continue to {next.Label}";
        CanGoBack = SetupSteps.Before(_steps, current) is not null;
        BackCommand.Refresh();
        ContinueCommand.Refresh();
        RetryCommand.Refresh();

        var drawn = (ShowsFields ? 1 : 0) + (ShowsQuality ? 1 : 0) + (ShowsReview ? 1 : 0);

        Assert.That(Steps.Count == _steps.Count, "a chip per step", Steps.Count, _steps.Count);
        Assert.That(drawn == 1, "the main column draws exactly one step form", ShowsFields, ShowsQuality, ShowsReview);
        Assert.That(
            !ShowsFields || CurrentGroup is not null || _steps.Count == 0,
            "a fields step on a resolved form has a group to draw", current);
        Assert.That(CanContinue == (ContinueLabel.Length > 0), "the continue button and its label agree", CanContinue, ContinueLabel);
        Assert.That(
            form is not null || _groups.Values.All(group => !group.IsResolved),
            "a flow with no form draws no group", _groups.Count);
        Assert.That(
            !Review.CanGoLive || IsPublishable,
            "the commit is offered only on settings the form said publish", IsPublishable);
        Assert.That(!Review.CanGoLive || !_starting, "one start is asked for at a time", _starting);
    }

    /// <summary>
    /// Which step the reader is actually standing on: the one they picked while the form
    /// still carries it, and the first step otherwise.
    ///
    /// The fallback is not a repair of the input. A form can drop the group the reader was
    /// on - it is the backend's list and it moves - and rewriting <see cref="CurrentStep"/>
    /// from a render pass would be the render function editing its own input. Reading it
    /// through instead means the reader lands back where they were if the group returns.
    /// </summary>
    private string Standing(IReadOnlyList<SetupStepRow> steps)
    {
        if (SetupSteps.Of(steps, CurrentStep) is not null)
        {
            return CurrentStep;
        }

        return steps.Count > 0 ? steps[0].Key : "";
    }

    /// <summary>
    /// Converges the backend onto the draft: asks for the form this draft resolves to,
    /// unless it has already been asked for it.
    ///
    /// <b>Idempotent, and that is what makes it safe on a render pass.</b> The contract
    /// states the resolve is side-effect free and answers the same form for the same draft
    /// (docs/ipc-api.md), so a draft that still equals the one last handed over has nothing
    /// to learn from a second round trip - whether that first answer has landed or is still
    /// in flight. Rendering twice therefore costs one call, not two, and rendering a hundred
    /// times costs the same one.
    /// </summary>
    private void Resolve()
    {
        // Before the stored settings arrive there is no draft to describe, and the read that
        // fetches them is already in flight from the constructor.
        if (_draft is null || _draft.Equals(_asked))
        {
            return;
        }

        // A copy, because the controls write the draft in place: handing the live instance
        // over would let the next keystroke change the message while it is being sent, and
        // would leave the answer describing settings nobody ever asked about.
        var draft = _draft.Clone();
        _asked = draft;
        Settled = Start(draft);
    }

    /// <summary>
    /// Starts one read and supersedes whatever was in flight. The token asks the older call
    /// to stop; the request number it stamps is what settles the race the token can lose.
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
    /// One read, off the UI thread. It writes nothing itself: the answer goes back through
    /// the dispatcher to <see cref="Adopt"/>, which is the only place the form and the draft
    /// are assigned.
    /// </summary>
    /// <param name="draft">
    /// The draft to resolve, or null on the first read, where the stored settings are the
    /// draft and fetching them is the hop in front of it.
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
            // A newer draft superseded this one. Its answer is the one the screen wants, and
            // this call ending is the point of having cancelled it.
        }
        catch (BackendUnavailableException e)
        {
            _dispatch(() => Fail(e.Message, request));
        }
    }

    /// <summary>
    /// Takes one answer, on the UI loop. The only write of <c>_form</c>, <c>_draft</c> and
    /// <c>_asked</c>, and the only place a render pass is triggered by something other than
    /// an input.
    ///
    /// <b>The latest answer wins.</b> Cancellation is cooperative: a call can already have
    /// produced its form by the time the token is set, so an answer to a draft the reader
    /// has moved off can still arrive, and arrive after a newer one. The request number is
    /// what makes that harmless rather than rare - an answer that is not the one the flow is
    /// waiting for is dropped, and the newer form stands.
    /// </summary>
    private void Adopt(Form form, int request)
    {
        Assert.NotNull(form, "a resolve answers with the form it resolved");

        if (request != _request)
        {
            return;
        }

        _form = form;
        _unreachable = "";

        // Adopted wholesale rather than merged: where the backend walked a forbidden value
        // to a legal one, the merge would be the flow deciding which half to keep.
        //
        // The draft is a copy of it and the form keeps its own, because the controls write
        // the draft in place and a write reaching into the form would edit the answer the
        // screen is drawing. The form's copy is what the next pass compares against, so a
        // repaired draft counts as asked about and settles here rather than costing a second
        // round trip - which the contract's idempotency is exactly the promise of.
        _draft = form.Settings.Clone();
        _asked = form.Settings;

        Apply();
    }

    /// <summary>
    /// Takes one refusal, on the UI loop. The screen keeps whatever form it was drawing and
    /// gains the sentence saying why there is no newer one, which is the honest pair: the last
    /// answer the backend gave is still the last answer it gave.
    ///
    /// <b>It leaves <c>_asked</c> where it was, and that is load-bearing.</b> Clearing it would
    /// mean the render pass this triggers finds a draft the backend has not been asked about,
    /// starts a resolve, fails, renders again - a loop that would hammer an absent socket for
    /// as long as the window is open. Asking again is <see cref="Retry"/>, which the reader
    /// runs when they have something new to expect.
    /// </summary>
    private void Fail(string reason, int request)
    {
        Assert.That(reason.Length > 0, "a read that could not be answered says why");

        if (request != _request)
        {
            return;
        }

        _unreachable = reason;
        Apply();
    }

    /// <summary>
    /// Asks the backend again for the draft on screen, because what it would answer has moved.
    /// The encoder probe landing is what raises that today: the forms resolved before it grey
    /// nothing for missing hardware, and the ones after it do.
    ///
    /// Clearing <c>_asked</c> is the whole of it. The draft is unchanged, so the round-trip
    /// guard would otherwise skip the read as one already answered - which it is, against
    /// facts that have since changed.
    /// </summary>
    private void Reask()
    {
        _asked = null;
        Apply();
    }

    /// <summary>
    /// Asks again after a failure. Two cases, because the first read has no draft in front of
    /// it: with settings in hand this is <see cref="Reask"/>, and without them it is the
    /// opening read started over.
    /// </summary>
    private void Retry()
    {
        _unreachable = "";

        if (_draft is null)
        {
            Settled = Start(draft: null);
            Apply();
            return;
        }

        Reask();
    }

    // --- The uplink measurement ------------------------------------------------------

    /// <summary>
    /// Measures the line and writes what it finds into the uplink field, which re-resolves
    /// the form and reprices everything beside it.
    ///
    /// It is an effect rather than a read: the backend uploads a payload, it takes seconds,
    /// and it is refused outright while a stream is publishing. So it is started by a press
    /// and never from a render pass, and the flag it sets is what keeps a second press from
    /// starting a second upload.
    /// </summary>
    private void Measure()
    {
        if (_measuring || _draft is null)
        {
            return;
        }

        _measuring = true;
        Apply();
        _ = MeasureAsync();
    }

    private async Task MeasureAsync()
    {
        try
        {
            var mbps = await _backend.MeasureUplinkAsync().ConfigureAwait(false);
            _dispatch(() => Measured(mbps));
        }
        catch (BackendUnavailableException e)
        {
            // Refusing to measure while a stream is live arrives here too, carrying the
            // backend's own sentence, which is the one worth showing.
            _dispatch(() => MeasureFailed(e.Message));
        }
    }

    /// <summary>
    /// Takes the measured figure, on the UI loop. It goes in through the same write every
    /// control uses, so the measurement is a value the reader could have typed rather than a
    /// second path into the draft.
    /// </summary>
    private void Measured(double mbps)
    {
        _measuring = false;

        if (_draft is null)
        {
            Apply();
            return;
        }

        Write(RailLayout.UplinkKey, new FieldValue { Number = (long)Math.Round(mbps) });
    }

    private void MeasureFailed(string reason)
    {
        Assert.That(reason.Length > 0, "a measurement that could not be made says why");

        _measuring = false;
        _unreachable = reason;
        Apply();
    }

    /// <summary>
    /// One field write, arriving from whichever control the reader moved. The draft is
    /// changed and the whole thing re-resolved: which other controls that frees or greys is
    /// the backend's answer, and asking for it is cheap by contract.
    /// </summary>
    private void Write(string key, FieldValue value)
    {
        // A control the reader can move was drawn from a form, and a form was resolved from a
        // draft, so a write arriving without one means a field was rendered from nothing.
        var draft = Assert.NotNull(_draft, "a control the reader moved was drawn from a draft");

        SettingsDraft.Write(draft, key, value);
        Apply();
    }

    /// <summary>The form's group under this key, or null where the form carries none.</summary>
    private static FieldGroup? GroupOf(Form form, string key)
    {
        foreach (var group in form.Groups)
        {
            if (group.Key == key)
            {
                return group;
            }
        }

        return null;
    }

    /// <summary>
    /// The uplink control, wherever the form put it, or null where it offers none. Looked up
    /// rather than held, because which group carries it is the backend's arrangement and the
    /// rail draws the same field view model that group's own step does (Model/RailLayout.cs).
    /// </summary>
    private FieldViewModel? Uplink()
    {
        foreach (var group in _groups.Values)
        {
            if (group.Visible(RailLayout.UplinkKey) is { } field)
            {
                return field;
            }
        }

        return null;
    }

    /// <summary>
    /// Each group's key, heading and shorthand, which is what the review reads back. Empty
    /// before the first form, so the review draws no tiles rather than four invented ones.
    /// </summary>
    private IReadOnlyList<(string Key, string Title, string Summary)> Summaries(Form? form)
        => form is null
            ? []
            : form.Groups
                .Select(group => (
                    group.Key,
                    Title: Copy.Fields.Group(group.Key).Title,
                    Summary: _session.Words.Shorthand(group.Key, form.Settings)))
                .ToList();

    /// <summary>
    /// Names the step that owns one field key, for the diagnostics that carry one. It is the
    /// one thing this flow uses the field-to-group arrangement for, and it is placement: the
    /// contract says which control a diagnostic is about, and this side is the only one that
    /// knows which screen that control ended up on.
    /// </summary>
    private static Func<string, string> AnchorIn(IReadOnlyList<SetupStepRow> steps, Form? form)
    {
        if (form is null)
        {
            return _ => "";
        }

        var owner = new Dictionary<string, string>();
        foreach (var group in form.Groups)
        {
            if (SetupSteps.Of(steps, group.Key) is not { } step)
            {
                continue;
            }

            foreach (var field in group.Fields)
            {
                owner[field.Key] = $"step {step.Number} · {step.Label}";
            }
        }

        return key => key.Length > 0 && owner.TryGetValue(key, out var where) ? where : "";
    }

    /// <summary>
    /// What a chip says its step settled on. A form-driven step repeats its group's own
    /// summary; the terminal step repeats the rail's count of what is still owed.
    /// </summary>
    private string ValueOf(SetupStepRow row) => row.IsTerminal ? Rail.ChecksSummary : Group(row.Key).Summary;

    private static StepContent ContentOf(string key) => key switch
    {
        QualityLayout.GroupKey => StepContent.Quality,
        SetupSteps.GoLiveKey => StepContent.Review,
        _ => StepContent.Fields,
    };

    /// <summary>The renderer for one group key, made on first use and kept.</summary>
    private FieldGroupViewModel Group(string key)
    {
        Assert.That(key.Length > 0, "a group renderer is identified by the group it draws");

        if (_groups.TryGetValue(key, out var group))
        {
            return group;
        }

        group = new FieldGroupViewModel(Write);
        _groups[key] = group;
        return group;
    }

    private DelegateCommand SelectCommandOf(string key)
    {
        Assert.That(key.Length > 0, "a select command is identified by the step it moves to");

        if (_select.TryGetValue(key, out var command))
        {
            return command;
        }

        command = new DelegateCommand(() => GoTo(key));
        _select[key] = command;
        return command;
    }

    /// <summary>The one write that moves the flow. Every chip and every Edit link ends here.</summary>
    private void GoTo(string key) => CurrentStep = key;

    private void Back()
    {
        var previous = Assert.NotNull(
            SetupSteps.Before(_steps, Standing(_steps)),
            "the back button moves off a step that has one before it");

        GoTo(previous.Key);
    }

    private void Continue()
    {
        var next = Assert.NotNull(
            SetupSteps.After(_steps, Standing(_steps)),
            "the continue button moves off a step that has one after it");

        GoTo(next.Key);
    }

    // --- The commit ------------------------------------------------------------------

    /// <summary>
    /// Starts the stream on the draft the reader configured.
    ///
    /// It is an effect and is therefore started by a press and never from a render pass, the
    /// same arrangement <see cref="Measure"/> has. What it hands over is a copy: the controls
    /// write the draft in place, so passing the live instance would let a keystroke change the
    /// settings while they are being sent and leave a stream running something nobody asked for.
    ///
    /// Nothing about the running state is written here. The reply says nothing and the stream
    /// that resulted arrives on the event stream, which is the one path into the display - so
    /// the window that pressed the button and the window that did not show the same thing
    /// (<c>docs/ipc-api.md</c>, "Events").
    /// </summary>
    private void GoLive()
    {
        // The command is already gated on the same conditions; this is the guard that holds when
        // the press and a state change race, which the round trip makes a real interval.
        if (_starting || _draft is null)
        {
            return;
        }

        _starting = true;
        _refusal = "";
        Apply();

        _ = GoLiveAsync(_draft.Clone());
    }

    private async Task GoLiveAsync(Settings settings)
    {
        try
        {
            await _backend.StartPublishAsync(settings).ConfigureAwait(false);
            _dispatch(Started);
        }
        catch (BackendUnavailableException e)
        {
            // A backend that refused - a stream already in force, a combination no engine can
            // build - arrives here carrying its own sentence, which is the one worth showing.
            _dispatch(() => StartFailed(e.Message));
        }
        catch (OperationCanceledException)
        {
            // Nothing cancels this call, since it carries no token. A transport that reports one
            // anyway still has to leave the button pressable rather than locked forever.
            _dispatch(() => StartFailed(""));
        }
    }

    /// <summary>
    /// Takes an accepted start, on the UI loop. The flow is rendered before the news goes out,
    /// so a listener that moves the window finds a screen that has already unlocked.
    /// </summary>
    private void Started()
    {
        _starting = false;
        _refusal = "";
        Apply();

        WentLive?.Invoke();
    }

    private void StartFailed(string reason)
    {
        _starting = false;
        _refusal = reason;
        Apply();
    }
}
