using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.Model;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.AdvancedDrawer.ViewModel;
using ScreenShare.App.Features.Setup.CostRail.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.QualityStep.ViewModel;
using ScreenShare.App.Features.Setup.ReviewStep.ViewModel;
using ScreenShare.App.Features.Setup.StepStrip.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.ViewModel;

/// <summary>
/// The setup flow: which step is showing, and the strip that is at once its navigation, its
/// progress and its summary.
///
/// <b>It owns neither the draft nor the form.</b> Both belong to <see cref="FormSession"/>,
/// which the window holds once, because the viewer edits settings too and a draft each would be
/// two copies of one message. This class reads that draft through on every pass and writes to
/// it; what it owns is the step the reader is standing on and the commit.
///
/// <b>It decides nothing else either.</b> Which steps exist, which controls each one draws,
/// what they are called, which values they offer, which of those are greyed and why, what the
/// configuration is predicted to cost and the line each step's chip carries all come back from
/// <see cref="IBackend.ResolveFormAsync"/> already decided (docs/ipc-api.md, "The rule").
///
/// <b>The steps are the form's groups, minus the one the viewer draws.</b> They used to be a
/// table here, and three of the seven rows named group keys the backend does not answer with -
/// so three steps of the wizard drew an empty column, and the four groups the table did not name
/// were unreachable. Deriving them removes the class of bug rather than the instance: a group
/// added to the contract is a step that appears and works with nothing here to edit
/// (<see cref="SetupSteps"/>). The one group held back is the watch group, and holding it back
/// is placement rather than a second list - this screen configures what this machine sends, and
/// how a stream comes back governs the tiles in the viewer (<see cref="GroupPlacement"/>).
///
/// <b>Inputs</b> are <see cref="CurrentStep"/> and the field writes that reach the draft through
/// <see cref="Write"/>. Both end in <see cref="Apply"/>.
///
/// <b>Outputs</b> are written by <see cref="Apply"/> on every pass, including the branches that
/// turn a form off. A flow whose draft has not resolved yet is an honest state rather than a
/// gap - the strip is empty, every group renders its unresolved branch, and the sentence saying
/// why sits above the column.
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
    /// The draft and the form it resolves to, owned by the window and read through here on every
    /// pass. This flow holds no copy of either.
    /// </summary>
    private readonly FormSession _form;

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

    /// <summary>
    /// The measurement the uplink figure is offered beside. The command is held and the action
    /// around it is made per pass, because what the action says moves with the running state:
    /// holding the command is what keeps the button the reader presses and the lock that refuses
    /// a second press one object (<see cref="FieldAction"/>).
    /// </summary>
    private readonly PendingCommand _measure;

    // --- What the screen is drawn from ---------------------------------------------

    /// <summary>
    /// Whether a commit this flow asked for is still in flight, read from the command that
    /// started it rather than mirrored in a field of its own. It locks the commit for exactly
    /// as long as the round trip lasts, so a second press cannot ask for a second stream - or a
    /// second restart of one - while the backend is still deciding about the first, and it is
    /// the same field the button draws its spinner from, so the lock and the wait cannot
    /// disagree.
    /// </summary>
    private bool Starting => Review.GoLiveCommand.IsRunning;

    /// <summary>
    /// Why the backend refused the last commit, empty otherwise. It is that side's own sentence
    /// and is shown as it stands: a refusal is prose written for a person
    /// (<c>docs/ipc-api.md</c>, "Errors").
    /// </summary>
    private string _refusal = "";

    /// <summary>
    /// What the backend answered about the last measurement, empty otherwise. It is that side's
    /// own sentence and rides on the button that asked for it, which is the control it is about -
    /// a measurement that did not happen leaves the form drawing perfectly well, so it is neither
    /// the banner that says the screen could not be described nor a panel at the foot of the
    /// column (<see cref="MeasureNotice"/>).
    /// </summary>
    private string _measured = "";

    /// <summary>
    /// The steps the last pass rendered. Held because moving through them is an input rather
    /// than a render - Back and Continue need the order, and the order is the form's.
    /// </summary>
    private IReadOnlyList<SetupStepRow> _steps = [];

    /// <param name="dispatch">
    /// Hands work to the UI loop. Injected rather than reached for, so this type stays free
    /// of a toolkit and a test can pass a synchronous dispatcher - the same arrangement
    /// <see cref="Backend.Session"/> uses, and for the same reason: the answer to an effect
    /// arrives on whichever thread the transport completed on, and every property below is read
    /// by a binding that only tolerates being written from one.
    /// </param>
    public SetupViewModel(IBackend backend, FormSession form, Session session, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a setup flow asks the backend to put its draft on the air");
        Assert.NotNull(form, "a setup flow draws the draft the window is holding");
        Assert.NotNull(session, "a setup flow reads the running state the commit turns on");
        Assert.NotNull(dispatch, "a setup flow needs a UI loop to marshal an answer back to");

        _backend = backend;
        _form = form;
        _session = session;
        _dispatch = dispatch;

        // Everything the render function reads is built before anything can call it, so a
        // step moved from a child's constructor would still find a complete view model.
        Steps = [];
        BackCommand = new DelegateCommand(Back, () => CanGoBack);
        ContinueCommand = new DelegateCommand(Continue, () => CanContinue);

        // Looking again is a read across the socket like every other, so the button waits on it
        // rather than sitting still while a backend that is coming up is dialled.
        RetryCommand = new PendingCommand(_form.RetryAsync, dispatch, () => IsUnavailable);

        // The measurement is an effect: it uploads a real payload, takes seconds, and the backend
        // refuses it outright while a stream is publishing. Hence a button the reader presses,
        // beside the figure it writes, rather than a number that fills itself in - and one that
        // is greyed in the state the backend refuses, rather than pressable into a refusal.
        _measure = new PendingCommand(
            MeasureAsync, dispatch, () => _form.Draft is not null && MeasureRefusal().Length == 0);

        // News that the draft or the form behind it moved, which is the one thing this flow
        // draws from. Raised on the UI loop by the form session itself, so there is nothing to
        // marshal here.
        _form.Changed += Apply;

        // The one group with a layout of its own, made eagerly because two children hold it.
        // Which controls it draws is still the form's answer - the step picks its fields out
        // of the group by key, and picking a place for a field is placement.
        Quality = new QualityStepViewModel(Group(QualityLayout.GroupKey));

        // The same group, and the other half of it: the drawer draws the fields the step's
        // layout places nowhere, so between them every control the backend offered is
        // reachable exactly once (Model/QualityLayout.cs).
        Advanced = new AdvancedDrawerViewModel(Group(QualityLayout.GroupKey));

        Rail = new CostRailViewModel();
        Review = new ReviewStepViewModel(SelectCommandOf, Back, GoLiveAsync, dispatch);

        // Both edges of an effect this flow renders: a start locks the commit and a measurement
        // greys the button that asked for it, and neither is a state anything else here would
        // notice moving. The commands own the fact and say when it moved; what it looks like is
        // still one pass.
        Review.GoLiveCommand.Changed += Apply;
        _measure.Changed += Apply;

        // Rendered before anything is asked for, so the window has a complete view model to
        // paint whether or not the backend is reachable, and the first form is a later pass
        // rather than a precondition of the first one.
        Apply();
    }

    /// <summary>
    /// Raised once the backend has accepted a commit this flow asked for - a start, or an apply
    /// onto the stream that was already running. It is news that the commit went through and
    /// carries nothing, in the way every other signal here does: what the stream became arrives
    /// on the event stream, and the window reads it there.
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
    private string _unsaved = "";
    private bool _hasUnsaved;
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
    /// because a retry loop would hammer an absent socket for as long as the window is open.
    ///
    /// It is no longer the only way back, and that is the point of keeping it narrow: a
    /// backend that comes back is noticed by <see cref="FormSession"/>, off the connection the
    /// window already holds. What is left for the button is the failure nothing else reports -
    /// a read the backend served a refusal to, or one that failed while the session's own reads
    /// did not.
    /// </summary>
    public PendingCommand RetryCommand { get; }

    /// <summary>
    /// The read in flight, and an already-completed task when none is. Read through from the
    /// form session rather than held, for the one caller that legitimately needs it: something
    /// that has to know the screen has caught up with the draft rather than merely having been
    /// asked to. A test waits on it instead of sleeping; nothing in the render path touches it.
    /// </summary>
    public Task Settled => _form.Settled;

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

    /// <summary>
    /// Why the last write to an applied group could not be stored, empty while they are being
    /// stored. It is the backend's own sentence, read through from the form session.
    ///
    /// It sits above the steps beside the unavailable banner rather than in it, because the two
    /// are different news: a read that cannot be answered leaves the screen showing an older
    /// answer and blocks the publish, and a write that cannot be stored leaves the screen showing
    /// exactly what the reader typed while the backend goes on running on the value before it
    /// (<see cref="FormSession.Unsaved"/>).
    /// </summary>
    public string Unsaved { get => _unsaved; private set => Set(ref _unsaved, value); }

    public bool HasUnsaved { get => _hasUnsaved; private set => Set(ref _hasUnsaved, value); }

    /// <summary>Names the next step rather than saying "Next", so the button says where it goes.</summary>
    public string ContinueLabel { get => _continueLabel; private set => Set(ref _continueLabel, value); }

    /// <summary>
    /// The one render function. It is synchronous, and it draws the last form the backend
    /// answered with rather than waiting for a newer one: asking is the form session's
    /// <see cref="FormSession.Sync"/>, whose answer arrives on a later pass.
    ///
    /// Safe to run twice: the converge it asks for is skipped when the draft has not moved,
    /// and every row it produces compares equal to the last one, so an unchanged pass fires
    /// no binding.
    /// </summary>
    public void Apply()
    {
        // Reconciled from the render pass rather than performed by it: the pass states what
        // it wants and the converge decides whether anything has to be asked
        // (docs/development-principles.md, "Idempotency").
        _form.Sync();

        // Read through once, so every output below is derived from one form rather than from
        // whatever the session held at the moment each of them was written.
        var form = _form.Form;
        var drawn = Drawn(form);

        // A renderer per group this screen draws, then a pass over every renderer this flow
        // holds - including the ones the newest form dropped, which is what makes them clear
        // rather than go on showing what an older form said.
        foreach (var group in drawn)
        {
            Group(group.Key);
        }

        foreach (var key in _groups.Keys.ToList())
        {
            _groups[key].Apply(GroupOf(drawn, key), _session.Words, form?.Settings);
        }

        IReadOnlyList<Diagnostic> diagnostics = form is null ? [] : form.Diagnostics;

        _steps = SetupSteps.For(drawn);
        var current = Standing(_steps);

        // The rail before the strip: the terminal chip repeats the rail's own summary, so it
        // has to be the summary of the list the rail is about to draw rather than of the one
        // it drew last pass.
        var checks = PreflightChecks.Of(diagnostics, AnchorIn(_steps, form));
        Rail.Apply(form?.Summary?.Estimate, Uplink(), SetupSteps.Of(_steps, GroupOwning(drawn, RailLayout.UplinkKey)), checks);

        IsPublishable = form?.Publishable ?? false;

        // Composed on every pass out of four states nothing here owns: the form's own verdict on
        // the settings, whether the backend answered at all, whether a stream is already in
        // force, and whether the relay is there to publish to. Three of them decide whether the
        // button lights; the stream in force decides what pressing it does, which is the gate's
        // Commit and is what the label and the sentence under it are read from. All four are
        // read through rather than cached, so a relay that came back unlocks the button on the
        // next pass, and a stream that ended puts the word "restart" back to "go live", without
        // anything having had to remember either.
        var gate = PublishGate.Of(IsPublishable, _form.Unavailable, _session.Publish, _session.Relay, Starting);
        Review.Apply(gate, _form.Draft?.Publish?.Name ?? "", _refusal, Summaries(drawn, form), checks);

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
        Unavailable = _form.Unavailable;
        IsUnavailable = Unavailable.Length > 0;

        // A notice and not the unavailable banner, which blocks the publish: settings that could
        // not be stored are still settings a stream can be started on.
        Unsaved = _form.Unsaved;
        HasUnsaved = Unsaved.Length > 0;

        var next = SetupSteps.After(_steps, current);
        CanContinue = next is not null;
        ContinueLabel = next is null ? "" : $"Continue to {next.Label}";
        CanGoBack = SetupSteps.Before(_steps, current) is not null;
        BackCommand.Refresh();
        ContinueCommand.Refresh();
        RetryCommand.Refresh();
        _measure.Refresh();

        var forms = (ShowsFields ? 1 : 0) + (ShowsQuality ? 1 : 0) + (ShowsReview ? 1 : 0);

        Assert.That(Steps.Count == _steps.Count, "a chip per step", Steps.Count, _steps.Count);
        Assert.That(forms == 1, "the main column draws exactly one step form", ShowsFields, ShowsQuality, ShowsReview);
        Assert.That(
            !ShowsFields || CurrentGroup is not null || _steps.Count == 0,
            "a fields step on a resolved form has a group to draw", current);
        Assert.That(CanContinue == (ContinueLabel.Length > 0), "the continue button and its label agree", CanContinue, ContinueLabel);
        Assert.That(
            form is not null || _groups.Values.All(group => !group.IsResolved),
            "a flow with no form draws no group", _groups.Count);
        Assert.That(
            _steps.All(step => step.IsTerminal || GroupPlacement.InSetup(step.Key)),
            "every step draws a group this screen places", _steps.Count);
        Assert.That(
            !Review.CanGoLive || IsPublishable,
            "the commit is offered only on settings the form said publish", IsPublishable);
        Assert.That(!Review.CanGoLive || !Starting, "one start is asked for at a time", Starting);
    }

    /// <summary>
    /// The groups this screen draws: every group the form carries except the ones another
    /// destination places. Empty for a form that has not arrived, which is what makes an
    /// unresolved flow draw an empty strip rather than steps it made up.
    /// </summary>
    private static IReadOnlyList<FieldGroup> Drawn(Form? form)
        => form is null ? [] : form.Groups.Where(group => GroupPlacement.InSetup(group.Key)).ToList();

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

    // --- The uplink measurement ------------------------------------------------------

    /// <summary>
    /// Measures the line and writes what it finds into the uplink field, which re-resolves
    /// the form and reprices everything beside it.
    ///
    /// It is an effect rather than a read: the backend uploads a payload, it takes seconds,
    /// and it is refused outright while a stream is publishing. So it is started by a press and
    /// never from a render pass, and the command that starts it is what keeps a second press
    /// from starting a second upload while the first is still going.
    ///
    /// The answer is marshalled back by hand. <see cref="PendingCommand"/> marshals its own
    /// completion and nothing else, so the continuation here runs on whichever thread the
    /// transport finished on - and everything below this line writes bound properties.
    /// </summary>
    private async Task MeasureAsync()
    {
        // What the last attempt said goes now rather than when this one answers, the same way the
        // commit's refusal does: it is about an attempt that is over, and leaving it up would put
        // a sentence about the last measurement beside a spinner about this one.
        _measured = "";
        Apply();

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
        catch (OperationCanceledException)
        {
            // Nothing cancels this call, since it carries no token. A transport that reports one
            // anyway still has to leave the button pressable rather than locked forever.
            _dispatch(() => MeasureFailed(""));
        }
    }

    /// <summary>
    /// Takes the measured figure, on the UI loop. It goes in through the same write every
    /// control uses, so the measurement is a value the reader could have typed rather than a
    /// second path into the draft.
    /// </summary>
    private void Measured(double mbps)
    {
        _measured = "";

        if (_form.Draft is null)
        {
            Apply();
            return;
        }

        Write(RailLayout.UplinkKey, new FieldValue { Number = (long)Math.Round(mbps) });
    }

    private void MeasureFailed(string reason)
    {
        _measured = reason;
        Apply();
    }

    /// <summary>
    /// Why the measurement cannot be taken now, empty while it can.
    ///
    /// <b>The state is the backend's and the sentence is this side's.</b> Whether a pipeline is in
    /// force is <c>PublishState.live</c>, read through the one derivation that owns the reading
    /// (<see cref="PublishGate.CommitFor"/>) rather than looked at again here. What the backend
    /// would answer is a refusal to a call the button need not make: an upload beside a live
    /// stream measures the line minus the stream, so the figure would be a property of the moment
    /// wearing the shape of a property of the machine.
    ///
    /// Read on demand rather than held, so a stream that ended unlocks the button on the next
    /// pass with nothing here having remembered that it was locked.
    /// </summary>
    private string MeasureRefusal()
        => PublishGate.CommitFor(_session.Publish) == PublishCommit.Apply
            ? "A stream is publishing, and measuring the line would compete with it. Stop the stream to measure."
            : "";

    /// <summary>
    /// What the button carries beside it: why it is greyed where it is, and what the last
    /// attempt answered otherwise. The refusal comes first, because a stream on the air is the
    /// state the reader is in rather than news about an attempt that is over.
    /// </summary>
    private string MeasureNotice()
        => MeasureRefusal() is { Length: > 0 } refusal ? refusal : _measured;

    /// <summary>
    /// One field write, arriving from whichever control the reader moved. It goes to the one
    /// owner of the draft, which re-resolves and announces - and this flow re-renders off that
    /// announcement like any other reader, rather than by knowing it had just written.
    /// </summary>
    private void Write(string key, FieldValue value) => _form.Write(key, value);

    /// <summary>The group under this key among the ones this screen draws, or null where there is none.</summary>
    private static FieldGroup? GroupOf(IReadOnlyList<FieldGroup> groups, string key)
    {
        foreach (var group in groups)
        {
            if (group.Key == key)
            {
                return group;
            }
        }

        return null;
    }

    /// <summary>
    /// The group that carries one field, or empty where none of the drawn ones does. It is what
    /// lets the rail name the step a figure is edited on without holding a second idea of where
    /// the backend put it.
    /// </summary>
    private static string GroupOwning(IReadOnlyList<FieldGroup> groups, string fieldKey)
    {
        foreach (var group in groups)
        {
            foreach (var field in group.Fields)
            {
                if (field.Key == fieldKey)
                {
                    return group.Key;
                }
            }
        }

        return "";
    }

    /// <summary>
    /// The uplink control, wherever the form put it, or null where it offers none. Looked up
    /// rather than held, because which group carries it is the backend's arrangement and the
    /// rail reads the same field view model that group's own step draws (Model/RailLayout.cs).
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
    /// before the first form, so the review draws no tiles rather than four invented ones, and
    /// it lists the groups this screen draws so the review and the strip name the same steps.
    /// </summary>
    private IReadOnlyList<(string Key, string Title, string Summary)> Summaries(
        IReadOnlyList<FieldGroup> groups, Form? form)
        => form is null
            ? []
            : groups
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
    ///
    /// A diagnostic about a control another destination draws anchors nowhere and says so by
    /// answering empty, which is the honest answer: the check is still listed, and it names no
    /// step of this wizard because no step of this wizard fixes it.
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

    /// <summary>
    /// The renderer for one group key, made on first use and kept. Every one of them is handed
    /// the same action lookup, so the measurement follows the uplink field to whichever group
    /// the backend puts it in rather than being nailed to one step.
    /// </summary>
    private FieldGroupViewModel Group(string key)
    {
        Assert.That(key.Length > 0, "a group renderer is identified by the group it draws");

        if (_groups.TryGetValue(key, out var group))
        {
            return group;
        }

        group = new FieldGroupViewModel(Write, ActionFor);
        _groups[key] = group;
        return group;
    }

    /// <summary>
    /// What this screen offers beside one control. One field has one: the uplink, which is a
    /// figure this machine can measure rather than only a figure to type.
    ///
    /// The action is composed here on every pass and the command inside it is the held one, so
    /// two passes over one state produce actions that compare equal while what the button says
    /// still follows the state that decides it (<see cref="FieldAction"/>).
    /// </summary>
    private FieldAction? ActionFor(string key) => key == RailLayout.UplinkKey
        ? new FieldAction(
            "Measure",
            "Uploads a short payload to measure this machine's real upload throughput, and puts the result in the box. Refused while a stream is live.",
            MeasureNotice(),
            _measure)
        : null;

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
    /// Puts the draft the reader configured on the air: it starts a stream where none is
    /// running, and restarts the running one on these settings where there is.
    ///
    /// <b>Which of the two it is, is read from the running state on this pass rather than
    /// remembered.</b> The gate the last render composed is not consulted - the stream can start
    /// or end between the pass that drew the button and the press that took it, and the backend
    /// refuses each of the two effects in exactly the state the other one is for. One derivation
    /// answers both sides, so what the label promised and what the press sends cannot come apart
    /// (<see cref="PublishGate.CommitFor"/>).
    ///
    /// It is an effect and is therefore started by a press and never from a render pass, the
    /// same arrangement <see cref="MeasureAsync"/> has. What it hands over is a copy: the
    /// controls write the draft in place, so passing the live instance would let a keystroke
    /// change the settings while they are being sent and leave a stream running something
    /// nobody asked for.
    ///
    /// <b>The copy is taken before the first await</b>, which is on the UI loop, so the draft it
    /// carries is the one that was on screen when the button went down rather than whatever it
    /// had become by the time the transport got to it. The reading of what is publishing is
    /// taken there too, and for the same reason.
    ///
    /// Nothing about the running state is written here. The reply says nothing and the stream
    /// that resulted arrives on the event stream, which is the one path into the display - so
    /// the window that pressed the button and the window that did not show the same thing
    /// (<c>docs/ipc-api.md</c>, "Events").
    /// </summary>
    private async Task GoLiveAsync()
    {
        // The command offers the press only on a gate that says these settings publish, and
        // nothing says that before a form has resolved a draft.
        var draft = Assert.NotNull(_form.Draft, "a commit that was offered was drawn from a draft");
        var settings = draft.Clone();
        var commit = PublishGate.CommitFor(_session.Publish);

        // The refusal the last attempt left goes now rather than when this one answers: it is
        // about an attempt that is over, and leaving it up would put a sentence about the last
        // commit beside a spinner about this one.
        _refusal = "";
        Apply();

        try
        {
            await CommitAsync(commit, settings).ConfigureAwait(false);
            _dispatch(Committed);
        }
        catch (BackendUnavailableException e)
        {
            // A backend that refused - a combination no engine can build, a stream that ended
            // between this pass and the call reaching the other side - arrives here carrying its
            // own sentence, which is the one worth showing.
            _dispatch(() => CommitFailed(e.Message));
        }
        catch (OperationCanceledException)
        {
            // Nothing cancels this call, since it carries no token. A transport that reports one
            // anyway still has to leave the button pressable rather than locked forever.
            _dispatch(() => CommitFailed(""));
        }
    }

    /// <summary>
    /// The one effect this commit is, for the state it was pressed in.
    ///
    /// Exhaustive, so a commit the gate learns to name and this does not fails here rather than
    /// quietly starting a second stream (<c>docs/development-principles.md</c>, "Contracts").
    /// </summary>
    private Task CommitAsync(PublishCommit commit, Settings settings) => commit switch
    {
        PublishCommit.Start => _backend.StartPublishAsync(settings),
        PublishCommit.Apply => _backend.ApplyToStreamAsync(settings),
        _ => Assert.Never<Task>("unexpected commit", (int)commit),
    };

    /// <summary>
    /// Takes an accepted commit, on the UI loop. The flow is rendered before the news goes out,
    /// so a listener that moves the window finds a screen that has already unlocked.
    /// </summary>
    private void Committed()
    {
        _refusal = "";
        Apply();

        WentLive?.Invoke();
    }

    private void CommitFailed(string reason)
    {
        _refusal = reason;
        Apply();
    }
}
