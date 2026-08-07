using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.ReviewStep.ViewModel;

/// <summary>
/// The last step: everything resolved, read back in one place, and the one control that
/// changes the world.
///
/// <b>Inputs</b> are the two things that happen the moment publishing starts. They are
/// asked here rather than after going live because each of them is a decision about the
/// stream that is about to exist, and a dialog that interrupts a live stream to ask is
/// worse than a switch that was already set.
///
/// <b>Outputs</b> are written by <see cref="Apply"/> alone, and every one of them comes off
/// a state some other side stated. The tiles are the groups' own shorthands, the list is the
/// form's diagnostics, the name is the draft's, and whether the button lights is the
/// <see cref="PublishGate"/> - which reads <c>Form.publishable</c>, what is publishing and
/// what the relay said, so the button and the refusal that would follow it cannot disagree.
/// </summary>
public sealed class ReviewStepViewModel : Observable
{
    private readonly Func<string, DelegateCommand> _edit;

    /// <param name="edit">Hands a command that moves the flow to one step. The review edits nothing itself.</param>
    /// <param name="back">Moves to the step before this one, which is the flow's own answer rather than a key held here.</param>
    /// <param name="goLive">
    /// What committing means. Owned above this view model - there is no publisher here to call,
    /// and starting one is an effect on the control plane this step has no seam to.
    /// </param>
    public ReviewStepViewModel(Func<string, DelegateCommand> edit, Action back, Action goLive)
    {
        Assert.NotNull(edit, "the review hands editing back to the step that owns it");
        Assert.NotNull(back, "the review needs the flow's own way back");
        Assert.NotNull(goLive, "the review hands the commit to whoever owns publishing");

        _edit = edit;
        Tiles = [];
        Checks = [];

        GoLiveCommand = new DelegateCommand(goLive, () => CanGoLive);
        BackCommand = new DelegateCommand(back);

        Apply(PublishGate.Unread, streamName: "", refusal: "", [], []);
    }

    // --- Inputs -------------------------------------------------------------------

    private bool _openBroadcastWindow = true;
    private bool _saveAsPreset;

    public bool OpenBroadcastWindow
    {
        get => _openBroadcastWindow;
        set => Set(ref _openBroadcastWindow, value);
    }

    public bool SaveAsPreset
    {
        get => _saveAsPreset;
        set => Set(ref _saveAsPreset, value);
    }

    // --- Outputs ------------------------------------------------------------------

    private bool _canGoLive;
    private string _blocked = "";
    private bool _isBlocked;
    private string _refusal = "";
    private bool _hasRefusal;
    private string _streamName = "";
    private bool _hasStreamName;

    public ObservableCollection<ReviewTile> Tiles { get; }

    /// <summary>The same list the rail carries, which is the form's diagnostics.</summary>
    public ObservableCollection<PreflightCheckRow> Checks { get; }

    public DelegateCommand GoLiveCommand { get; }

    public DelegateCommand BackCommand { get; }

    /// <summary>
    /// Whether the settings can be published as they stand, and everything else the commit
    /// depends on holds. The gate's own answer, so the one red button and the refusal that
    /// would follow it are one decision.
    /// </summary>
    public bool CanGoLive { get => _canGoLive; private set => Set(ref _canGoLive, value); }

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

        CanGoLive = gate.CanGoLive;
        Blocked = gate.Blocked;
        IsBlocked = Blocked.Length > 0;

        Refusal = refusal;
        HasRefusal = Refusal.Length > 0;

        StreamName = streamName;
        HasStreamName = StreamName.Length > 0;

        GoLiveCommand.Refresh();

        Assert.That(Tiles.Count == groups.Count, "a tile per group of the form", Tiles.Count, groups.Count);
        Assert.That(!CanGoLive || !IsBlocked, "a commit that is offered has nothing standing in its way", Blocked);
        Assert.That(IsBlocked == (Blocked.Length > 0), "the locked notice and its sentence agree", IsBlocked);
        Assert.That(HasRefusal == (Refusal.Length > 0), "a refusal and its sentence agree", HasRefusal);
    }
}
