using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.Presets.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.ReviewStep.ViewModel;

/// <summary>
/// The last step: everything resolved, read back in one place, and the one control that
/// changes the world.
///
/// <b>It composes the preset card</b> (<see cref="Presets"/>), which is the one thing on this
/// screen that is neither a reading of the settings nor the commit. It sits here because a
/// preset is the whole way of publishing, and this is the screen where the whole way of
/// publishing is read back: naming one and picking one belong beside that summary rather than
/// on a step that owns a fraction of it.
///
/// <b>Outputs</b> are written by <see cref="Apply"/> alone, and every one of them comes off
/// a state some other side stated. The tiles are the groups' own shorthands, the list is the
/// form's diagnostics, the name is the draft's, and whether the button lights is the
/// <see cref="PublishGate"/> - which reads <c>Form.publishable</c>, what is publishing and
/// what the relay said, so the button and the refusal that would follow it cannot disagree.
///
/// <b>What the button says is the same gate's answer, and that is the point of it being one.</b>
/// A stream already on the air no longer locks the commit; it decides which effect the press is
/// (<see cref="PublishGate.Commit"/>), and the label and the sentence under it are that answer
/// read out of one table. The alternative was a ternary at the binding site and a second one
/// beside the call, which is one fact written down twice and free to drift.
/// </summary>
public sealed class ReviewStepViewModel : Observable
{
    private readonly Func<string, DelegateCommand> _edit;

    /// <param name="edit">Hands a command that moves the flow to one step. The review edits nothing itself.</param>
    /// <param name="back">Moves to the step before this one, which is the flow's own answer rather than a key held here.</param>
    /// <param name="goLive">
    /// What committing means, and it answers when the backend has answered. Owned above this
    /// view model - there is no publisher here to call, and starting one is an effect on the
    /// control plane this step has no seam to.
    /// </param>
    /// <param name="presets">The saved ways of publishing, drawn in this step's own column.</param>
    /// <param name="dispatch">The UI loop the commit's answer is marshalled back to.</param>
    public ReviewStepViewModel(
        Func<string, DelegateCommand> edit,
        Action back,
        Func<Task> goLive,
        PresetsViewModel presets,
        Action<Action> dispatch)
    {
        Assert.NotNull(edit, "the review hands editing back to the step that owns it");
        Assert.NotNull(back, "the review needs the flow's own way back");
        Assert.NotNull(goLive, "the review hands the commit to whoever owns publishing");
        Assert.NotNull(presets, "the review draws the saved ways of publishing beside the one being reviewed");
        Assert.NotNull(dispatch, "the review needs a UI loop to marshal the commit's answer back to");

        _edit = edit;
        Presets = presets;
        Tiles = [];
        Checks = [];

        // A start crosses to the backend, which persists the settings and launches an encoder on
        // them, so the button waits rather than going inert: the round trip is long enough for a
        // reader to press again, and the command is what refuses that.
        StartSharingCommand = new PendingCommand(goLive, dispatch, () => CanStartSharing);
        BackCommand = new DelegateCommand(back);

        Apply(PublishGate.Unread, streamName: "", refusal: "", [], []);
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

    public ObservableCollection<ReviewTile> Tiles { get; }

    /// <summary>The same list the rail carries, which is the form's diagnostics.</summary>
    public ObservableCollection<PreflightCheckRow> Checks { get; }

    /// <summary>
    /// The saved ways of publishing. Composed rather than owned: it reads the store and writes
    /// the draft through seams of its own, and this step only decides where on the screen it
    /// sits and renders it on every pass.
    /// </summary>
    public PresetsViewModel Presets { get; }

    /// <summary>
    /// The commit. Its own in-flight field is what the button draws its wait from, so the
    /// spinner on screen and the press the command would refuse are one fact.
    /// </summary>
    public PendingCommand StartSharingCommand { get; }

    public DelegateCommand BackCommand { get; }

    /// <summary>
    /// Whether the settings can be published as they stand, and everything else the commit
    /// depends on holds. The gate's own answer, so the one red button and the refusal that
    /// would follow it are one decision.
    /// </summary>
    public bool CanStartSharing { get => _canStartSharing; private set => Set(ref _canStartSharing, value); }

    /// <summary>
    /// What the button says it will do: start a stream, or restart the one already on the air on
    /// these settings. It comes off the gate's <see cref="PublishGate.Commit"/> through one table
    /// (<see cref="CommitCopy"/>), so the word on the control and the call the press makes are
    /// the same answer read twice rather than two answers that happen to agree.
    /// </summary>
    public string CommitLabel { get => _commitLabel; private set => Set(ref _commitLabel, value); }

    /// <summary>
    /// The promise under the button, up to the stream name. It is two halves because the name
    /// sits inside the sentence at full strength, and the sentence itself is what tells a reader
    /// that applying restarts the stream rather than changing it under the viewers.
    /// </summary>
    public string PromiseLead { get => _promiseLead; private set => Set(ref _promiseLead, value); }

    /// <summary>The rest of that sentence, after the name.</summary>
    public string PromiseTail { get => _promiseTail; private set => Set(ref _promiseTail, value); }

    /// <summary>
    /// Why the button is locked, empty while it is not. It is never this step's own prose about
    /// a setting: the settings' own blockers are in <see cref="Checks"/>, in the words the
    /// backend wrote them in.
    /// </summary>
    public string Blocked { get => _blocked; private set => Set(ref _blocked, value); }

    public bool IsBlocked { get => _isBlocked; private set => Set(ref _isBlocked, value); }

    /// <summary>
    /// The backend's own sentence when it refused the last start, empty otherwise. A refusal is
    /// about an attempt rather than about a precondition, which is why it is a second line and
    /// not folded into <see cref="Blocked"/>.
    /// </summary>
    public string Refusal { get => _refusal; private set => Set(ref _refusal, value); }

    public bool HasRefusal { get => _hasRefusal; private set => Set(ref _hasRefusal, value); }

    /// <summary>
    /// The stream's path on the relay, as the draft carries it. It used to be a name out of the
    /// mockups printed under every configuration, which said the same thing whatever the reader
    /// had typed.
    /// </summary>
    public string StreamName { get => _streamName; private set => Set(ref _streamName, value); }

    public bool HasStreamName { get => _hasStreamName; private set => Set(ref _hasStreamName, value); }

    /// <summary>
    /// The one render function. Idempotent: the rows are rebuilt from the arguments and
    /// reconciled, and two passes over one form produce rows that compare equal.
    /// </summary>
    /// <param name="gate">Whether the commit is available, and why it is not.</param>
    /// <param name="streamName">The draft's stream name, empty before a draft has arrived.</param>
    /// <param name="refusal">What the backend said when it refused the last start, empty otherwise.</param>
    public void Apply(
        PublishGate gate,
        string streamName,
        string refusal,
        IReadOnlyList<(string Key, string Title, string Summary)> groups,
        IReadOnlyList<PreflightCheckRow> checks)
    {
        Assert.NotNull(gate, "the review draws the gate the flow composed");
        Assert.NotNull(groups, "the review draws the groups the form carried");
        Assert.NotNull(checks, "the review draws the list the form's diagnostics became");

        Reconcile.Onto(Tiles, ReviewTiles.Of(groups, _edit));
        Reconcile.Onto(Checks, checks);

        // Read out of the one table on every pass, including the branch that puts the label back
        // to a start: a stream that ended has to take the word "restart" off the button with it,
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

        StartSharingCommand.Refresh();

        // The card draws from the draft and from the store, neither of which this step holds, so
        // it is rendered rather than fed: what it reads has moved by the time this pass runs.
        Presets.Apply();

        Assert.That(Tiles.Count == groups.Count, "a tile per group of the form", Tiles.Count, groups.Count);
        Assert.That(CommitLabel.Length > 0, "the commit says what pressing it will do", gate.Commit);
        Assert.That(
            PromiseLead.Length > 0 && PromiseTail.Length > 0,
            "the promise carries both halves of the sentence the name sits in", gate.Commit);
        Assert.That(!CanStartSharing || !IsBlocked, "a commit that is offered has nothing standing in its way", Blocked);
        Assert.That(IsBlocked == (Blocked.Length > 0), "the locked notice and its sentence agree", IsBlocked);
        Assert.That(HasRefusal == (Refusal.Length > 0), "a refusal and its sentence agree", HasRefusal);
    }
}
