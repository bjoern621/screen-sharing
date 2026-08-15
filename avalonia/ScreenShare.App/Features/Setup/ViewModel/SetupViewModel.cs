using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.Model;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.AdvancedDrawer.ViewModel;
using ScreenShare.App.Features.Setup.CostRail.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.Presets.ViewModel;
using ScreenShare.App.Features.Setup.QualityStep.ViewModel;
using ScreenShare.App.Features.Setup.ReviewStep.ViewModel;
using ScreenShare.App.Features.Setup.ScreenPicker.ViewModel;
using ScreenShare.App.Features.Setup.StepStrip.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.ViewModel;

/// <summary>
/// The setup flow: which step is showing, and the strip that is its navigation, its progress and its summary
/// at once.
///
/// <b>It owns neither the draft nor the form.</b> Both belong to <see cref="FormSession"/>, which the window
/// holds once, because the viewer edits settings too and a draft each would be two copies of one message.
/// This flow reads that draft through on every pass and writes to it; what it owns is the step the reader
/// stands on and the commit.
///
/// <b>It decides nothing else either.</b> Which steps exist, which controls each draws, what they are called,
/// which values they offer, which of those are greyed and why, what the configuration is predicted to cost and
/// the line each step's chip carries all arrive decided from <see cref="IBackend.ResolveFormAsync"/>
/// (docs/ipc-api.md, "The rule").
///
/// <b>The steps are the form's groups, minus the one the viewer draws.</b> Deriving them is what makes a group
/// added to the contract a step that appears and works with nothing here to edit (<see cref="SetupSteps"/>).
/// The group held back is the watch group, and holding it back is placement rather than a second list: this
/// screen configures what this machine sends, and how a stream comes back governs the tiles in the viewer
/// (<see cref="GroupPlacement"/>).
///
/// <b>Inputs</b> are <see cref="CurrentStep"/> and the field writes that reach the draft through
/// <see cref="Write"/>, and both end in <see cref="Apply"/>.
///
/// <b>Outputs</b> are written by <see cref="Apply"/> on every pass, including the branches that turn a form
/// off.
/// A flow whose draft has not resolved is a state rather than a gap: the strip is empty, every group renders
/// its unresolved branch, and the sentence saying why sits above the column.
/// </summary>
public sealed class SetupViewModel : Observable
{
    /// <summary>
    /// Which form the main column draws.
    /// A split by layout and not by substance: no branch of it knows what its step's settings mean.
    /// </summary>
    private enum StepContent
    {
        /// <summary>One group of the resolved form, through the generic renderer.</summary>
        Fields,

        /// <summary>
        /// A group in a layout of its own: cards for the choice whose options carry a paragraph each, a banded
        /// track for the scale with named ends, and a row of dropdowns for what is read back off the source.
        /// </summary>
        Quality,

        /// <summary>The terminal step: what blocks the publish, rather than a setting to edit.</summary>
        Review,
    }

    private readonly IBackend _backend;

    /// <summary>
    /// The draft and the form it resolves to, owned by the window and read through on every pass.
    /// No copy of either is held here.
    /// </summary>
    private readonly FormSession _form;

    /// <summary>
    /// The running state, owned by the window and read through on every pass.
    /// No copy is held here: what is publishing and what the relay answered are among the states the commit
    /// turns on, and a second reading of either is a second opinion about it.
    /// </summary>
    private readonly Session _session;

    private readonly Action<Action> _dispatch;

    /// <summary>
    /// One select command per step key, made once and reused.
    /// Reusing the instance is what lets a chip row be a record: two passes over one step compare equal rather
    /// than merely look alike, and the strip is left alone.
    /// </summary>
    private readonly Dictionary<string, DelegateCommand> _select = [];

    /// <summary>
    /// One renderer per group key, made on demand and kept across passes.
    /// The form-driven steps differ in nothing this layer can see, so they are instances of one component.
    /// </summary>
    private readonly Dictionary<string, FieldGroupViewModel> _groups = [];

    /// <summary>
    /// One reset command per group key, made once and reused, for the reason the select commands are: the held
    /// instance is what lets the action beside a heading be a record that compares equal across passes.
    /// </summary>
    private readonly Dictionary<string, DelegateCommand> _reset = [];

    /// <summary>
    /// The measurement the uplink figure is offered beside.
    /// The command is held and the action around it is made per pass, since what the action says moves with the
    /// running state: holding the command keeps the button the reader presses and the lock refusing a second
    /// press one object (<see cref="FieldAction"/>).
    /// </summary>
    private readonly PendingCommand _measure;

    /// <summary>
    /// Drawing a group key, on the same terms as <see cref="_measure"/>: a round trip to a service on another
    /// machine, so the button waits on it rather than sitting still.
    /// </summary>
    private readonly PendingCommand _createGroup;

    // --- What the screen is drawn from ---------------------------------------------

    /// <summary>
    /// Whether a commit this flow asked for is still in flight, read off the command that started it rather
    /// than mirrored in a field.
    /// It locks the commit for as long as the round trip lasts, so a second press cannot ask for a second
    /// stream, or a second restart of one, while the backend is still deciding about the first.
    /// The button draws its spinner from the same field, so the lock and the wait cannot disagree.
    /// </summary>
    private bool Starting => Review.StartSharingCommand.IsRunning;

    /// <summary>
    /// Why the backend refused the last commit, empty otherwise.
    /// That side's own sentence, shown as it stands: a refusal is prose written for a person
    /// (docs/ipc-api.md, "Errors").
    /// </summary>
    private string _refusal = "";

    /// <summary>
    /// What the backend answered about the last measurement, empty otherwise.
    /// That side's own sentence, and it rides on the button that asked for it: a measurement that did not
    /// happen leaves the form drawing, so it is neither the banner about a screen that cannot be described nor
    /// a panel at the foot of the column (<see cref="MeasureNotice"/>).
    /// </summary>
    private string _measured = "";

    /// <summary>
    /// What the last attempt to draw a group key answered: the id of the group that was made, or the backend's
    /// sentence for why none was.
    /// The key itself is not here. It goes into the field the reader can already see, so what is shown beside
    /// the button is news about the attempt rather than a second copy of the secret.
    /// </summary>
    private string _groupDrawn = "";

    /// <summary>
    /// The steps the last pass rendered.
    /// Held because moving through them is an input rather than a render: Back and Continue need the order, and
    /// the order is the form's.
    /// </summary>
    private IReadOnlyList<SetupStepRow> _steps = [];

    /// <param name="dispatch">
    /// Hands work to the UI loop.
    /// Injected rather than reached for, so this type holds no toolkit and a test passes a synchronous
    /// dispatcher, the arrangement <see cref="Backend.Session"/> has for the same reason: an effect's answer
    /// arrives on whichever thread the transport completed on, and every property below is read by a binding
    /// that tolerates being written from one thread only.
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

        // Everything the render function reads exists before anything can call it, so a step moved from a
        // child's constructor still finds a complete view model.
        Steps = [];
        BackCommand = new DelegateCommand(Back, () => CanGoBack);
        ContinueCommand = new DelegateCommand(Continue, () => CanContinue);

        // Looking again is a read across the socket like any other, so the button waits on it rather than
        // sitting still while a backend that is coming up is dialled.
        RetryCommand = new PendingCommand(_form.RetryAsync, dispatch, () => IsUnavailable);

        // Measuring is an effect: a real payload goes up, it takes seconds, and the backend refuses it outright
        // while a stream is publishing.
        // Hence a button beside the figure it writes rather than a number that fills itself in, greyed in the
        // state the backend refuses rather than pressable into a refusal.
        _measure = new PendingCommand(
            MeasureAsync, dispatch, () => _form.Draft is not null && MeasureRefusal().Length == 0);

        // Drawing a key is an effect too, and one nothing else can do on the reader's behalf: a key this app
        // adopted by itself would put the stream in a group nobody else holds.
        // Pressable only where there is a service to draw from, which is a relay behind a TLS proxy.
        _createGroup = new PendingCommand(
            CreateGroupAsync, dispatch, () => _form.Draft is not null && _form.Draft.Relay is not null
                && _form.Draft.Relay.Tls && _form.Draft.Relay.Host.Length > 0);

        // News that the draft, or the form behind it, moved: the one thing this flow draws from.
        // Raised on the UI loop by the form session, so there is nothing to marshal here.
        _form.Changed += Apply;

        // The one group with a layout of its own, made eagerly because two children take it.
        // Which controls it draws is still the form's answer: the step picks its fields out of the group by
        // key, and picking a place for a field is placement.
        Quality = new QualityStepViewModel(Group(QualityLayout.GroupKey));

        // The same group's other half: the drawer draws the fields the step's layout places nowhere, so between
        // them every control the backend offered is reachable exactly once (Model/QualityLayout.cs).
        Advanced = new AdvancedDrawerViewModel(Group(QualityLayout.GroupKey));

        // The pictures over the source step's controls.
        // The screen it is told to pick goes through the seam every control writes through, so the grid and the
        // list beneath it are two ways to one value rather than two values.
        Screens = new ScreenPickerViewModel(
            backend, session, dispatch,
            monitor => Write(SourceLayout.MonitorKey, new FieldValue { Number = monitor }));

        // The rail, and the saved ways of publishing in it, handed the seams this flow reads and nothing of
        // this flow's own: the store is the backend's and the draft is the window's, so a card routed through
        // here would be one more hop between a press and the state it changes.
        // The rail draws on every step, so a preset is offered wherever the reader is standing
        // (CostRail/ViewModel/CostRailViewModel.cs).
        Rail = new CostRailViewModel(new PresetsViewModel(backend, form, session, dispatch));

        Review = new ReviewStepViewModel(SelectCommandOf, Back, StartSharingAsync, dispatch);

        // Both edges of an effect this flow renders: a start locks the commit and a measurement greys the button
        // that asked for it, and neither is a state anything else here would notice moving.
        // The commands own the fact and say when it moved, and what it looks like is still one pass.
        Review.StartSharingCommand.Changed += Apply;
        _measure.Changed += Apply;

        // Rendered before anything is asked for, so the window has a complete view model to paint whether or not
        // the backend is reachable, and the first form lands on a later pass rather than gating this one.
        Apply();
    }

    /// <summary>
    /// Raised once the backend has accepted a commit this flow asked for: a start, or an apply onto the stream
    /// already running.
    /// It carries nothing, as every signal here does, since what the stream became arrives on the event stream
    /// and the window reads it there.
    ///
    /// Whoever hosts this flow owns what happens next, because that is a change of destination and the
    /// destination is the window's state rather than this flow's.
    /// </summary>
    public event Action? WentLive;

    // --- Input --------------------------------------------------------------------

    private string _currentStep = "";

    /// <summary>
    /// The step showing, named by the form group it draws.
    /// The strip is non-linear on purpose: a returning reader clicks straight to the encode step and starts
    /// sharing, with nothing requiring the steps to be walked in order.
    ///
    /// Empty until the first form lands, and a key the newest form no longer carries is not an error: the
    /// render pass falls back to the first step rather than drawing nothing.
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
    private bool _canGoBack;
    private bool _canContinue;
    private bool _isPublishable;
    private string _continueLabel = "";
    private string _unavailable = "";
    private bool _isUnavailable;
    private bool _isDialling;
    private string _unsaved = "";
    private bool _hasUnsaved;
    private FieldGroupViewModel? _currentGroup;

    public QualityStepViewModel Quality { get; }

    public AdvancedDrawerViewModel Advanced { get; }

    /// <summary>
    /// The screens on offer, drawn from what is on them.
    /// Above the source step's controls and nowhere else, and drawing nothing where this machine cannot show
    /// what one screen holds.
    /// </summary>
    public ScreenPickerViewModel Screens { get; }

    public CostRailViewModel Rail { get; }

    public ReviewStepViewModel Review { get; }

    public ObservableCollection<StepChipViewModel> Steps { get; }

    public DelegateCommand BackCommand { get; }

    public DelegateCommand ContinueCommand { get; }

    /// <summary>
    /// Asks again after the backend could not answer.
    /// A command rather than a timer, because a retry loop would hammer an absent socket for as long as the
    /// window is open.
    ///
    /// It is not the only way back, which is the point of keeping it narrow: a backend that comes back is
    /// noticed by <see cref="FormSession"/>, off the connection the window already holds.
    /// What is left for the button is the failure nothing else reports, a read the backend served a refusal to
    /// or one that failed while the session's own reads did not.
    /// </summary>
    public PendingCommand RetryCommand { get; }

    /// <summary>
    /// The read in flight, an already-completed task when none is.
    /// Read through from the form session rather than held, for the one caller that needs it: something that
    /// has to know the screen has caught up with the draft rather than merely been asked to.
    /// A test waits on it instead of sleeping, and nothing in the render path touches it.
    /// </summary>
    public Task Settled => _form.Settled;

    /// <summary>The renderer the step showing draws through, null on a step that draws something else.</summary>
    public FieldGroupViewModel? CurrentGroup { get => _currentGroup; private set => Set(ref _currentGroup, value); }

    public bool ShowsFields { get => _showsFields; private set => Set(ref _showsFields, value); }

    public bool ShowsQuality { get => _showsQuality; private set => Set(ref _showsQuality, value); }

    /// <summary>
    /// The terminal step, which draws its read-back in the step column and its commit at the foot of the rail
    /// where the other steps' Back and Continue sit.
    /// </summary>
    public bool ShowsReview { get => _showsReview; private set => Set(ref _showsReview, value); }

    public bool CanGoBack { get => _canGoBack; private set => Set(ref _canGoBack, value); }

    public bool CanContinue { get => _canContinue; private set => Set(ref _canContinue, value); }

    /// <summary>
    /// Whether the settings publish as they stand.
    /// Stated by the form rather than ranked here, so the button and the refusal are one answer.
    /// False while no form has arrived, the honest reading of settings nothing has vouched for.
    /// </summary>
    public bool IsPublishable { get => _isPublishable; private set => Set(ref _isPublishable, value); }

    /// <summary>
    /// Why the backend could not describe the screen, empty while it can.
    /// The backend's own sentence, shown as it stands: a shell with nothing to talk to says so rather than
    /// drawing a form it made up (docs/ipc-api.md, "What each side owes").
    /// </summary>
    public string Unavailable { get => _unavailable; private set => Set(ref _unavailable, value); }

    public bool IsUnavailable { get => _isUnavailable; private set => Set(ref _isUnavailable, value); }

    /// <summary>
    /// Whether the window is still dialling behind the banner.
    /// Drawn as motion and not a countdown: a second counted down is a number nobody acts on.
    /// Beside the retry button rather than instead of it, the button being the reader asking for the attempt now
    /// (<c>Features/Setup/View/SetupView.axaml</c>).
    /// </summary>
    public bool IsDialling { get => _isDialling; private set => Set(ref _isDialling, value); }

    /// <summary>
    /// Why the last write to an applied group could not be stored, empty while they are being stored.
    /// The backend's own sentence, read through from the form session.
    ///
    /// It sits above the steps beside the unavailable banner rather than inside it, because the two are
    /// different news: a read that cannot be answered leaves an older answer on screen and blocks the publish,
    /// and a write that cannot be stored leaves exactly what the reader typed on screen while the backend goes
    /// on running on the value before it (<see cref="FormSession.Unsaved"/>).
    /// </summary>
    public string Unsaved { get => _unsaved; private set => Set(ref _unsaved, value); }

    public bool HasUnsaved { get => _hasUnsaved; private set => Set(ref _hasUnsaved, value); }

    /// <summary>Names the step it goes to rather than saying "Next".</summary>
    public string ContinueLabel { get => _continueLabel; private set => Set(ref _continueLabel, value); }

    /// <summary>
    /// The one render function.
    /// Synchronous, and drawing the last form the backend answered with rather than waiting for a newer one:
    /// asking is the form session's <see cref="FormSession.Sync"/>, whose answer arrives on a later pass.
    ///
    /// Safe to run twice: the converge it asks for is skipped while the draft has not moved, and every row it
    /// produces compares equal to the last, so an unchanged pass fires no binding.
    /// </summary>
    public void Apply()
    {
        // Reconciled from the render pass rather than performed by it: the pass names what it wants and the
        // converge decides whether anything has to be asked (docs/development-principles.md, "Idempotency").
        _form.Sync();

        // Read through once, so every output below derives from one form rather than from whatever the session
        // held at the moment each was written.
        var form = _form.Form;
        var drawn = Drawn(form);

        // A renderer per group this screen draws, then a pass over every renderer held, including the ones the
        // newest form dropped: that is what clears them rather than leaving an older form's answer on screen.
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

        // The rail before the strip: the terminal chip repeats the rail's summary, so that summary has to be of
        // the list the rail is about to draw rather than of the one it drew last pass.
        var checks = PreflightChecks.Of(diagnostics, AnchorIn(_steps, form));
        Rail.Apply(
            form?.Summary?.Estimate,
            Uplink(),
            SetupSteps.Of(_steps, GroupOwning(drawn, RailLayout.UplinkKey)),
            checks,
            form?.Summary?.CommandError ?? "");

        IsPublishable = form?.Publishable ?? false;

        // Composed on every pass out of states nothing here owns: the form's verdict on the settings, whether
        // the backend answered at all, whether a stream is already in force, and whether the relay is there to
        // publish to.
        // A stream in force decides what pressing the button does rather than whether it lights, which is the
        // gate's Commit and what the label and the sentence under it are read from; the rest decide the
        // lighting.
        // Every one of them is read through rather than cached, so a relay that came back unlocks the button on
        // the next pass and a stream that ended puts "restart" back to "start sharing", with nothing having had
        // to remember either.
        var gate = PublishGate.Of(IsPublishable, _form.Unavailable, _session.Publish, _session.Relay, Starting);
        Review.Apply(gate, _form.Draft?.Publish?.Name ?? "", _refusal, Summaries(drawn, form));

        Reconcile.Onto(Steps, StepChips.For(_steps, current, ValueOf, SelectCommandOf));

        var content = ContentOf(current);
        ShowsFields = content == StepContent.Fields;
        ShowsQuality = content == StepContent.Quality;
        ShowsReview = content == StepContent.Review;
        CurrentGroup = ShowsFields && current.Length > 0 ? Group(current) : null;

        // The pictures over the source step.
        // Which screen setting they are about is read out of the form like every other control, and whether they
        // are drawn at all is the picker's own converge: it opens a screen capture per monitor, so it is told
        // which step the reader stands on rather than left to draw whenever the flow renders.
        Screens.Apply(
            FieldOf(GroupOf(drawn, SourceLayout.GroupKey), SourceLayout.MonitorKey),
            current == SourceLayout.GroupKey);

        Unavailable = _form.Unavailable;
        IsUnavailable = Unavailable.Length > 0;

        // Read off the session's verdict and not the banner's sentence: a refusal the backend served is a read
        // that failed with the socket up and nothing being dialled after it.
        IsDialling = IsUnavailable && _session.Unavailable.Length > 0;

        // A notice and not the unavailable banner, which blocks the publish: settings that could not be stored
        // are still settings a stream starts on.
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
        _createGroup.Refresh();

        var forms = (ShowsFields ? 1 : 0) + (ShowsQuality ? 1 : 0) + (ShowsReview ? 1 : 0);

        Assert.That(Steps.Count == _steps.Count, "a chip per step", Steps.Count, _steps.Count);
        Assert.That(forms == 1, "the main column draws exactly one step form", ShowsFields, ShowsQuality, ShowsReview);
        Assert.That(
            !ShowsFields || CurrentGroup is not null || _steps.Count == 0,
            "a fields step on a resolved form has a group to draw", current);
        Assert.That(CanContinue == (ContinueLabel.Length > 0), "the continue button and its label agree", CanContinue, ContinueLabel);
        Assert.That(!IsDialling || IsUnavailable, "the wait appears beside the banner it belongs to", IsDialling, IsUnavailable);
        Assert.That(
            form is not null || _groups.Values.All(group => !group.IsResolved),
            "a flow with no form draws no group", _groups.Count);
        Assert.That(
            _steps.All(step => step.IsTerminal || GroupPlacement.InSetup(step.Key)),
            "every step draws a group this screen places", _steps.Count);
        Assert.That(
            !Review.CanStartSharing || IsPublishable,
            "the commit is offered only on settings the form said publish", IsPublishable);
        Assert.That(!Review.CanStartSharing || !Starting, "one start is asked for at a time", Starting);
    }

    /// <summary>
    /// The groups this screen draws: every group the form carries except the ones another destination places.
    /// Empty for a form that has not arrived, which is what makes an unresolved flow draw an empty strip rather
    /// than steps it invented.
    /// </summary>
    private static IReadOnlyList<FieldGroup> Drawn(Form? form)
        => form is null ? [] : form.Groups.Where(group => GroupPlacement.InSetup(group.Key)).ToList();

    /// <summary>
    /// The step the reader is standing on: the one picked while the form still carries it, the first step
    /// otherwise.
    ///
    /// The fallback is not a repair of the input.
    /// A form can drop the group the reader was on, since it is the backend's list and it moves, and rewriting
    /// <see cref="CurrentStep"/> from a render pass would be the render function editing its own input.
    /// Reading through instead lands the reader back where they were if the group returns.
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
    /// Measures the line and writes what it finds into the uplink field, which re-resolves the form and reprices
    /// everything beside it.
    ///
    /// An effect rather than a read: the backend uploads a payload, it takes seconds, and it is refused outright
    /// while a stream is publishing.
    /// So it is started by a press and never from a render pass, and the command that starts it keeps a second
    /// press from starting a second upload over the first.
    ///
    /// The answer is marshalled back by hand.
    /// <see cref="PendingCommand"/> marshals its own completion and nothing else, so the continuation here runs
    /// on whichever thread the transport finished on, and everything below this line writes bound properties.
    /// </summary>
    private async Task MeasureAsync()
    {
        // What the last attempt said goes now rather than when this one answers, as the commit's refusal does:
        // it is about an attempt that is over, and leaving it up would put a sentence about the last measurement
        // beside a spinner about this one.
        _measured = "";
        Apply();

        try
        {
            var mbps = await _backend.MeasureUplinkAsync().ConfigureAwait(false);
            _dispatch(() => Measured(mbps));
        }
        catch (BackendUnavailableException e)
        {
            // A refusal to measure beside a live stream arrives here too, carrying the backend's own sentence,
            // which is the one worth showing.
            _dispatch(() => MeasureFailed(e.Message));
        }
        catch (OperationCanceledException)
        {
            // This call carries no token, so nothing cancels it.
            // A transport reporting one anyway still has to leave the button pressable rather than locked for
            // good.
            _dispatch(() => MeasureFailed(""));
        }
    }

    /// <summary>
    /// Draws a group key and writes it to the field, on the terms <see cref="MeasureAsync"/> runs on.
    ///
    /// The key goes in through the write every control uses, so joining a group is a settings change the reader
    /// can see, undo and hand on, rather than something that happened to the machine.
    /// </summary>
    private async Task CreateGroupAsync()
    {
        _groupDrawn = "";
        Apply();

        var relay = _form.Draft?.Relay;
        if (relay is null)
        {
            return;
        }

        try
        {
            var group = await _backend.CreateGroupAsync(relay).ConfigureAwait(false);
            _dispatch(() => GroupDrawn(group.Key, group.Id));
        }
        catch (BackendUnavailableException e)
        {
            _dispatch(() => GroupFailed(e.Message));
        }
        catch (OperationCanceledException)
        {
            _dispatch(() => GroupFailed(""));
        }
    }

    /// <summary>Takes the drawn key, on the UI loop.</summary>
    private void GroupDrawn(string key, string id)
    {
        if (_form.Draft is null)
        {
            _groupDrawn = "";
            Apply();
            return;
        }

        _groupDrawn = $"Group {id} created. Everyone you give this key to can watch, and nobody else can.";
        Write(RelayLayout.GroupKeyKey, new FieldValue { Text = key });
    }

    private void GroupFailed(string reason)
    {
        _groupDrawn = reason;
        Apply();
    }

    /// <summary>
    /// Why no group can be drawn now, empty while one can.
    /// A relay reached without a TLS proxy runs no group service, so there is nothing to ask.
    /// </summary>
    private string GroupRefusal()
        => _form.Draft?.Relay is { Tls: true, Host.Length: > 0 }
            ? ""
            : "Groups are drawn by the relay's own service, which a relay reached without a TLS proxy does not run.";

    /// <summary>
    /// What the group button carries beside it: why it is greyed where it is, what the last attempt answered
    /// otherwise.
    /// </summary>
    private string GroupNotice()
        => GroupRefusal() is { Length: > 0 } refusal ? refusal : _groupDrawn;

    /// <summary>
    /// Takes the measured figure, on the UI loop.
    /// It goes in through the write every control uses, so the measurement is a value the reader could have
    /// typed rather than a second path into the draft.
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
    /// <b>The state is the backend's and the sentence is this side's.</b> Whether a pipeline is in force is
    /// <c>PublishState.live</c>, read through the one derivation that owns that reading
    /// (<see cref="PublishGate.CommitFor"/>) rather than looked at again here.
    /// The backend would refuse a call the button need not make: an upload beside a live stream measures the
    /// line minus the stream, so the figure would describe the moment while wearing the shape of a property of
    /// the machine.
    ///
    /// Read on demand rather than held, so a stream that ended unlocks the button on the next pass with nothing
    /// here having remembered it was locked.
    /// </summary>
    private string MeasureRefusal()
        => PublishGate.CommitFor(_session.Publish) == PublishCommit.Apply
            ? "A stream is publishing, and measuring the line would compete with it. Stop the stream to measure."
            : "";

    /// <summary>
    /// What the button carries beside it: why it is greyed where it is, what the last attempt answered
    /// otherwise.
    /// The refusal wins, because a stream on the air is the state the reader is in rather than news about an
    /// attempt that is over.
    /// </summary>
    private string MeasureNotice()
        => MeasureRefusal() is { Length: > 0 } refusal ? refusal : _measured;

    /// <summary>
    /// One field write, from whichever control the reader moved.
    /// It goes to the one owner of the draft, which re-resolves and announces, and this flow re-renders off that
    /// announcement like any other reader rather than off knowing it had written.
    /// </summary>
    private void Write(string key, FieldValue value) => _form.Write(key, value);

    /// <summary>
    /// One control of one group, null where the form carries neither.
    /// Null is an answer and not a gap: a form that has not arrived, and a backend that drew this group without
    /// this control, are both states the layout above has to render.
    /// </summary>
    private static Field? FieldOf(FieldGroup? group, string key)
    {
        if (group is null)
        {
            return null;
        }

        foreach (var field in group.Fields)
        {
            if (field.Key == key)
            {
                return field;
            }
        }

        return null;
    }

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
    /// The group carrying one field, empty where none of the drawn ones does.
    /// It is what lets the rail name the step a figure is edited on without a second idea of where the backend
    /// put it.
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
    /// The uplink control wherever the form put it, null where the form offers none.
    /// Looked up rather than held: which group carries it is the backend's arrangement, and the rail reads the
    /// same field view model that group's own step draws (Model/RailLayout.cs).
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
    /// Each group's key, heading and shorthand, which is what the review reads back.
    /// Empty before the first form, so the review draws no tiles rather than invented ones, and it lists the
    /// groups this screen draws so the review and the strip name the same steps.
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
    /// Names the step owning one field key, for the diagnostics that carry one.
    /// The one thing this flow uses the field-to-group arrangement for, and it is placement: the contract says
    /// which control a diagnostic is about, and this side is the only one that knows which screen that control
    /// ended up on.
    ///
    /// A diagnostic about a control another destination draws anchors nowhere and answers empty, which is the
    /// honest answer: the check is still listed, and it names no step here because no step here fixes it.
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
    /// What a chip says its step settled on.
    /// A form-driven step repeats its group's own summary, the terminal step the rail's count of what is still
    /// owed.
    /// </summary>
    private string ValueOf(SetupStepRow row) => row.IsTerminal ? Rail.ChecksSummary : Group(row.Key).Summary;

    private static StepContent ContentOf(string key) => key switch
    {
        QualityLayout.GroupKey => StepContent.Quality,
        SetupSteps.ShareKey => StepContent.Review,
        _ => StepContent.Fields,
    };

    /// <summary>
    /// The renderer for one group key, made on first use and kept.
    /// All of them are handed the same action lookup, so the measurement follows the uplink field to whichever
    /// group the backend puts it in rather than being nailed to one step.
    /// </summary>
    private FieldGroupViewModel Group(string key)
    {
        Assert.That(key.Length > 0, "a group renderer is identified by the group it draws");

        if (_groups.TryGetValue(key, out var group))
        {
            return group;
        }

        group = new FieldGroupViewModel(Write, ActionFor, GroupActionFor);
        _groups[key] = group;
        return group;
    }

    /// <summary>
    /// What this screen offers beside a heading: on an applied group, a reset to what a fresh installation
    /// holds.
    ///
    /// <b>Which groups those are is the form's answer rather than a name written here.</b> An applied group is
    /// one whose fields are the settings themselves, stored as they are typed and read by the backend on a
    /// schedule of its own (<c>form.proto</c>, FieldGroup.applied).
    /// A staged group is a proposal, so a reader who dislikes what they typed walks away and what this machine
    /// is has not moved; an applied one has already become what this machine is, and nothing else puts it back.
    /// Where the relay is, is the group that exists: a reader who changed a port has no other way back to the
    /// number the relay serves on.
    ///
    /// The action is composed per pass around the held command, so two passes over one form produce actions
    /// that compare equal (<see cref="GroupAction"/>).
    /// </summary>
    private GroupAction? GroupActionFor(FieldGroup group) => group.Applied
        ? new GroupAction(
            "Reset to defaults",
            "Puts every setting under this heading back to the value a fresh installation starts with.",
            ResetCommandOf(group.Key))
        : null;

    private DelegateCommand ResetCommandOf(string key)
    {
        Assert.That(key.Length > 0, "a reset command is identified by the group it puts back");

        if (_reset.TryGetValue(key, out var command))
        {
            return command;
        }

        command = new DelegateCommand(() => _form.Reset(key));
        _reset[key] = command;
        return command;
    }

    /// <summary>
    /// What this screen offers beside one control.
    /// The uplink is the field that has one, being a figure this machine can measure rather than only type.
    ///
    /// The action is composed on every pass and the command inside it is the held one, so two passes over one
    /// state produce actions that compare equal while what the button says still follows the state deciding it
    /// (<see cref="FieldAction"/>).
    /// </summary>
    private FieldAction? ActionFor(string key) => key switch
    {
        RailLayout.UplinkKey => new FieldAction(
            "Measure",
            "Uploads a short payload to measure this machine's real upload throughput, and puts the result in the box. Refused while a stream is live.",
            MeasureNotice(),
            _measure),
        RelayLayout.GroupKeyKey => new FieldAction(
            "Create group",
            "Draws a new group key at the relay and puts it in the box. Hand the key to the people who should be able to watch. Leaving the box empty publishes where anyone can watch.",
            GroupNotice(),
            _createGroup),
        _ => null,
    };

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

    /// <summary>The one write that moves the flow: every chip and every Edit link ends here.</summary>
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
    /// Puts the draft on the air: a stream is started where none runs, and the running one is restarted on
    /// these settings where one does.
    ///
    /// <b>Which of the two is read off the running state on this pass rather than remembered.</b> The gate the
    /// last render composed is not consulted, since the stream can start or end between the pass that drew the
    /// button and the press that took it, and the backend refuses each effect in exactly the state the other
    /// one is for.
    /// One derivation answers both sides, so what the label promised and what the press sends cannot come apart
    /// (<see cref="PublishGate.CommitFor"/>).
    ///
    /// An effect, so it is started by a press and never from a render pass, the arrangement
    /// <see cref="MeasureAsync"/> has.
    /// What it hands over is a copy: the controls write the draft in place, so passing the live instance would
    /// let a keystroke change the settings mid-send and leave a stream running something nobody asked for.
    ///
    /// <b>The copy is taken before the first await</b>, which is on the UI loop, so the draft it carries is the
    /// one on screen when the button went down rather than whatever it had become by the time the transport got
    /// to it.
    /// The reading of what is publishing is taken there too, for the same reason.
    ///
    /// No running state is written here.
    /// The reply says nothing and the resulting stream arrives on the event stream, the one path into the
    /// display, so the window that pressed the button and the window that did not show the same thing
    /// (docs/ipc-api.md, "Events").
    /// </summary>
    private async Task StartSharingAsync()
    {
        // The command offers the press only on a gate saying these settings publish, and nothing says that
        // before a form has resolved a draft.
        var draft = Assert.NotNull(_form.Draft, "a commit that was offered was drawn from a draft");
        var settings = draft.Clone();
        var commit = PublishGate.CommitFor(_session.Publish);

        // The refusal the last attempt left goes now rather than when this one answers: it is about an attempt
        // that is over, and leaving it up would put a sentence about the last commit beside a spinner about
        // this one.
        _refusal = "";
        Apply();

        try
        {
            await CommitAsync(commit, settings).ConfigureAwait(false);
            _dispatch(Committed);
        }
        catch (BackendUnavailableException e)
        {
            // A refusal, over a combination no engine can build or a stream that ended between this pass and
            // the call reaching the other side, arrives here carrying the backend's own sentence, which is the
            // one worth showing.
            _dispatch(() => CommitFailed(e.Message));
        }
        catch (OperationCanceledException)
        {
            // This call carries no token, so nothing cancels it.
            // A transport reporting one anyway still has to leave the button pressable rather than locked for
            // good.
            _dispatch(() => CommitFailed(""));
        }
    }

    /// <summary>
    /// The one effect this commit is, for the state it was pressed in.
    ///
    /// Exhaustive, so a commit the gate learns to name and this does not fails here rather than quietly starting
    /// a second stream (docs/development-principles.md, "Contracts").
    /// </summary>
    private Task CommitAsync(PublishCommit commit, Settings settings) => commit switch
    {
        PublishCommit.Start => _backend.StartPublishAsync(settings),
        PublishCommit.Apply => _backend.ApplyToStreamAsync(settings),
        _ => Assert.Never<Task>("unexpected commit", (int)commit),
    };

    /// <summary>
    /// Takes an accepted commit, on the UI loop.
    /// The flow renders before the news goes out, so a listener that moves the window finds a screen already
    /// unlocked.
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
