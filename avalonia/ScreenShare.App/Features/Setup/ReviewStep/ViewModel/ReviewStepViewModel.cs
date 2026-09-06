using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.ReviewStep.ViewModel;

/// <summary>
/// Last step: everything resolved, read back in one place, and the one control that changes the world.
///
/// The tiles draw in the step column and the commit at the foot of the rail, where every other step's Back
/// and Continue sit (<c>Setup/View/SetupView.axaml</c>).
/// What the settings owe and what has been saved are the rail's, on every step alike
/// (<c>Setup/CostRail/ViewModel/CostRailViewModel.cs</c>).
///
/// Outputs are written by <see cref="Apply"/> alone, each coming off a state some other side stated:
/// the tiles are the groups' own shorthands, the name is the draft's,
/// and whether the button lights is the <see cref="PublishGate"/>, reading <c>Form.publishable</c>, what is publishing
/// and what the relay said.
///
/// The word on the button is that same gate's answer (<see cref="PublishGate.Commit"/>),
/// a stream already on the air deciding which effect the press is rather than blocking it.
/// A ternary at the binding site and a second one beside the call would be one fact written down twice.
/// </summary>
public sealed class ReviewStepViewModel : Observable
{
    private readonly Func<string, DelegateCommand> _edit;

    /// <param name="edit">Hands back a command that moves the flow to one step. The review edits nothing.</param>
    /// <param name="back">Moves to the step before this one, the flow's answer rather than a key held here.</param>
    /// <param name="goLive">
    /// What committing means, answering when the backend has answered.
    /// Owned above this view model: no publisher here,
    /// and starting one is an effect on the control plane this step has no route to.
    /// </param>
    /// <param name="dispatch">UI loop the commit's answer is marshalled back to.</param>
    public ReviewStepViewModel(
        Func<string, DelegateCommand> edit,
        Action back,
        Func<Task> goLive,
        Action<Action> dispatch)
    {
        Assert.NotNull(edit, "the review hands editing back to the step that owns it");
        Assert.NotNull(back, "the review needs the flow's own way back");
        Assert.NotNull(goLive, "the review hands the commit to whoever owns publishing");
        Assert.NotNull(dispatch, "the review needs a UI loop to marshal the commit's answer back to");

        _edit = edit;
        Tiles = [];

        // A start crosses to the backend, which persists the settings and launches an encoder on them,
        // so the button waits rather than going inert: the round trip is long enough for a second press,
        // and the command is what refuses it.
        StartSharingCommand = new PendingCommand(goLive, dispatch, () => CanStartSharing);
        BackCommand = new DelegateCommand(back);

        Apply(PublishGate.Unread, streamName: "", refusal: "", []);
    }

    // --- Outputs ------------------------------------------------------------------

    private bool _canStartSharing;
    private string _commitLabel = "";
    private string _promiseLead = "";
    private string _promiseTail = "";
    private string _blocked = "";
    private bool _isBlocked;
    private string _refusal = "";
    private bool _hasRefusal;
    private string _streamName = "";
    private bool _hasStreamName;
    private bool _isInForce;
    private string _inForce = "";
    private bool _showsPromise;

    public ObservableCollection<ReviewTile> Tiles { get; }

    /// <summary>
    /// The commit.
    /// The button draws its wait from the command's own in-flight field,
    /// so a control that looks busy is a call that is out.
    /// </summary>
    public PendingCommand StartSharingCommand { get; }

    public DelegateCommand BackCommand { get; }

    /// <summary>
    /// Whether the settings can be published as they stand and everything else the commit depends on holds.
    /// The gate's own answer, so the button and the refusal that would follow pressing it cannot disagree.
    /// </summary>
    public bool CanStartSharing { get => _canStartSharing; private set => Set(ref _canStartSharing, value); }

    /// <summary>
    /// What the button says it will do: start a stream, or restart the one on the air on these settings.
    /// Read off <see cref="PublishGate.Commit"/> through <see cref="CommitCopy"/>, the same answer the press reads,
    /// rather than two answers that happen to agree.
    /// </summary>
    public string CommitLabel { get => _commitLabel; private set => Set(ref _commitLabel, value); }

    /// <summary>
    /// Promise under the button, up to the stream name.
    /// Two halves because the name sits inside the sentence at full strength,
    /// and the sentence tells a reader that applying restarts the stream rather than changing it under the viewers.
    /// </summary>
    public string PromiseLead { get => _promiseLead; private set => Set(ref _promiseLead, value); }

    /// <summary>Rest of that sentence, after the name.</summary>
    public string PromiseTail { get => _promiseTail; private set => Set(ref _promiseTail, value); }

    /// <summary>
    /// Why the button is locked, empty while it is not.
    /// Never this step's own prose about a setting: a setting's own blocker is in the rail's checks,
    /// in the words the backend wrote it in.
    /// </summary>
    public string Blocked { get => _blocked; private set => Set(ref _blocked, value); }

    public bool IsBlocked { get => _isBlocked; private set => Set(ref _isBlocked, value); }

    /// <summary>
    /// Backend's own sentence for a refused start, empty otherwise.
    /// A second line rather than folded into <see cref="Blocked"/>, being about an attempt rather than a precondition.
    /// </summary>
    public string Refusal { get => _refusal; private set => Set(ref _refusal, value); }

    public bool HasRefusal { get => _hasRefusal; private set => Set(ref _hasRefusal, value); }

    /// <summary>Stream's path on the relay, as the draft carries it.</summary>
    public string StreamName { get => _streamName; private set => Set(ref _streamName, value); }

    public bool HasStreamName { get => _hasStreamName; private set => Set(ref _hasStreamName, value); }

    /// <summary>
    /// Whether the stream runs the pipeline the draft builds, the gate's reading of <c>Form.in_force</c>.
    /// Greys the commit: a restart into the same pipeline costs every viewer the picture for nothing.
    /// </summary>
    public bool IsInForce { get => _isInForce; private set => Set(ref _isInForce, value); }

    /// <summary>The sentence for that state, drawn where the promise would be. Empty otherwise.</summary>
    public string InForce { get => _inForce; private set => Set(ref _inForce, value); }

    /// <summary>
    /// Whether the promise under the button draws: a named stream, and a press the card offers.
    /// The promise describes a press, so it gives way to <see cref="InForce"/> where none is offered.
    /// </summary>
    public bool ShowsPromise { get => _showsPromise; private set => Set(ref _showsPromise, value); }

    /// <summary>
    /// The one render function.
    /// Idempotent: the rows are rebuilt from the arguments and reconciled,
    /// so two passes over one form produce rows that compare equal.
    /// </summary>
    /// <param name="gate">Whether the commit is available, which effect it is, and why it is not.</param>
    /// <param name="streamName">Draft's stream name. Empty before a draft has arrived.</param>
    /// <param name="refusal">Backend's sentence for a refused start. Empty otherwise.</param>
    public void Apply(
        PublishGate gate,
        string streamName,
        string refusal,
        IReadOnlyList<(string Key, string Title, string Summary)> groups)
    {
        Assert.NotNull(gate, "the review draws the gate the flow composed");
        Assert.NotNull(groups, "the review draws the groups the form carried");

        Reconcile.Onto(Tiles, ReviewTiles.Of(groups, _edit));

        // Read out of the one table on every pass, the branch that puts the label back to a start included:
        // a stream that ended takes the word "restart" off the button with it,
        // and a property written only in the apply branch is one that sticks.
        var words = CommitCopy.Of(gate.Commit);
        CommitLabel = words.Label;
        PromiseLead = words.Lead;
        PromiseTail = words.Tail;

        CanStartSharing = gate.CanStartSharing;
        Blocked = gate.Blocked;
        IsBlocked = Blocked.Length > 0;

        Refusal = refusal;
        HasRefusal = Refusal.Length > 0;

        StreamName = streamName;
        HasStreamName = StreamName.Length > 0;

        // Written on every pass, the branch that puts the promise back included:
        // a stream that ended, or a value moved off it, offers the press again and the sentence about it.
        IsInForce = gate.InForce;
        InForce = gate.InForce ? words.InForce : "";
        ShowsPromise = HasStreamName && !IsInForce;

        StartSharingCommand.Refresh();

        Assert.That(Tiles.Count == groups.Count, "a tile per group of the form", Tiles.Count, groups.Count);
        Assert.That(CommitLabel.Length > 0, "the commit says what pressing it will do", gate.Commit);
        Assert.That(
            PromiseLead.Length > 0 && PromiseTail.Length > 0,
            "the promise carries both halves of the sentence the name sits in", gate.Commit);
        Assert.That(!CanStartSharing || !IsBlocked, "a commit that is offered has nothing standing in its way", Blocked);
        Assert.That(IsBlocked == (Blocked.Length > 0), "the locked notice and its sentence agree", IsBlocked);
        Assert.That(HasRefusal == (Refusal.Length > 0), "a refusal and its sentence agree", HasRefusal);
        Assert.That(IsInForce == (InForce.Length > 0), "the in-force notice and its sentence agree", gate.Commit);
        Assert.That(!ShowsPromise || !IsInForce, "the promise and the in-force notice take turns", IsInForce);
    }
}
