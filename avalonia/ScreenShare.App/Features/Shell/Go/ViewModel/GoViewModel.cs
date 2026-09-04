using System.Collections.ObjectModel;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.ViewModel;
using ScreenShare.App.Features.Setup.Presets.ViewModel;
using ScreenShare.App.Features.Setup.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.Go.ViewModel;

/// <summary>
/// Strip commit: the one control that starts sharing from any destination, and the menu beside it.
///
/// The button presses the review's own commit, so the label, the guard, the wait and the refusal surface
/// stay one each (<c>Features/Tray/ViewModel/TrayViewModel.cs</c> states the argument).
/// A preset row writes the draft through the rail card's own command and presses that same commit,
/// so one pick is on the air on the preset, live or not.
/// The summary line repeats the publish groups' shorthands, the derivation the review's tiles read.
///
/// Nothing here decides anything: which effect the press is comes off <see cref="Model.PublishGate.CommitFor"/>
/// through the review, and where "Open setup" leads is the shell's.
///
/// <see cref="Apply"/> is the one render function.
/// The shell's pass drives it for the session's state; the review and the draft announce their own moves,
/// so the strip follows a form resolved while another destination was showing.
/// </summary>
public sealed class GoViewModel : Observable
{
    /// <summary>
    /// Groups the summary line repeats, in the wizard's order.
    /// The publish half alone: relay and watch settings belong to this computer rather than to what it sends,
    /// the same cut the presets card states (<c>Copy/Cards.cs</c>, <c>PresetsCovers</c>).
    /// </summary>
    private static readonly IReadOnlyList<string> SummaryGroups = ["source", "quality", "audio", "transport"];

    private readonly Session _session;
    private readonly FormSession _form;
    private readonly SetupViewModel _setup;
    private readonly BroadcastViewModel _broadcast;
    private readonly Action _openSetup;

    /// <param name="openSetup">Moves the window to the wizard. The strip owns no destination.</param>
    public GoViewModel(
        Session session,
        FormSession form,
        SetupViewModel setup,
        BroadcastViewModel broadcast,
        Action openSetup)
    {
        Assert.NotNull(session, "the strip reads what is publishing off the session");
        Assert.NotNull(form, "the strip's summary reads the resolved form");
        Assert.NotNull(setup, "the strip presses the setup flow's own commit");
        Assert.NotNull(broadcast, "the strip's menu presses the broadcast screen's own stop");
        Assert.NotNull(openSetup, "the strip hands navigation to whoever owns the destination");

        _session = session;
        _form = form;
        _setup = setup;
        _broadcast = broadcast;
        _openSetup = openSetup;

        // The review re-renders on every form move and announces it, so the label and the gate follow
        // a draft edited on the wizard; the form's own signal covers a summary change the review's
        // outputs do not repeat.
        setup.Review.PropertyChanged += (_, _) => Apply();
        form.Changed += Apply;

        Apply();
    }

    /// <summary>
    /// The commit, the review's own instance.
    /// The button draws its wait and its guard from the command, so the strip and the wizard cannot disagree.
    /// </summary>
    public PendingCommand CommitCommand => _setup.Review.StartSharingCommand;

    /// <summary>The broadcast screen's own stop, for the menu's row while a stream is live.</summary>
    public PendingCommand StopCommand => _broadcast.StopCommand;

    /// <summary>The rail card's rows, read through. The menu lists what the card lists.</summary>
    public ObservableCollection<BuiltinPresetRow> Builtin => _setup.Rail.Presets.Builtin;

    public ObservableCollection<PresetRow> Saved => _setup.Rail.Presets.Rows;

    // --- Outputs -------------------------------------------------------------------

    private string _commitLabel = "";
    private string _blocked = "";
    private string _summary = "";
    private bool _isLive;

    /// <summary>What the press will do, the review's own answer: start, or restart the stream on the air.</summary>
    public string CommitLabel { get => _commitLabel; private set => Set(ref _commitLabel, value); }

    /// <summary>Why the button is locked, empty while it is not. The gate's sentence.</summary>
    public string Blocked { get => _blocked; private set => Set(ref _blocked, value); }

    private string? _blockedTip;

    /// <summary><see cref="Blocked"/> shaped for the tooltip: null while free, an empty tip drawing an empty bubble.</summary>
    public string? BlockedTip { get => _blockedTip; private set => Set(ref _blockedTip, value); }

    /// <summary>The publish groups' shorthands in one line, empty before the first form.</summary>
    public string Summary { get => _summary; private set => Set(ref _summary, value); }

    public bool IsLive { get => _isLive; private set => Set(ref _isLive, value); }

    // --- Presses -------------------------------------------------------------------

    public void OpenSetup() => _openSetup();

    /// <summary>
    /// Writes one built-in preset into the draft through the card's own row, and commits.
    /// One press is on the air on the preset; a stream already running restarts on it,
    /// the commit reading which off the running state.
    /// The row is looked up at the press, and one missing or out of reach does nothing,
    /// like it does on the card.
    /// </summary>
    public void UseBuiltin(string key)
    {
        Assert.That(key.Length > 0, "picking a preset names the row that was picked");

        var row = Builtin.FirstOrDefault(row => row.Key == key);
        if (row is null || !row.IsReachable)
        {
            return;
        }

        row.Apply.Execute(null);
        CommitCommand.Execute(null);
    }

    /// <summary>Same press for a saved preset: the card's apply, then the commit.</summary>
    public void UseSaved(string name)
    {
        Assert.That(name.Length > 0, "picking a preset names the row that was picked");

        var row = Saved.FirstOrDefault(row => row.Name == name);
        if (row is null)
        {
            return;
        }

        row.Apply.Execute(null);
        CommitCommand.Execute(null);
    }

    /// <summary>
    /// The one render function.
    /// Every output on every pass, so an unchanged pass writes no property and notifies nothing.
    /// </summary>
    public void Apply()
    {
        CommitLabel = _setup.Review.CommitLabel;
        Blocked = _setup.Review.Blocked;
        BlockedTip = Blocked.Length > 0 ? Blocked : null;
        IsLive = _session.Publish?.Live is not null;
        Summary = SummaryOf();

        Assert.That(CommitLabel.Length > 0, "the commit says what pressing it will do");
    }

    /// <summary>
    /// The shorthands joined the way the step chips print them.
    /// Groups with nothing worth a line drop out, so the line never carries a bare separator.
    /// </summary>
    private string SummaryOf()
    {
        var form = _form.Form;
        if (form is null)
        {
            return "";
        }

        var parts = SummaryGroups
            .Select(key => _session.Words.Shorthand(
                form.Groups.FirstOrDefault(group => group.Key == key), form.Settings))
            .Where(part => part.Length > 0);

        return string.Join(" · ", parts);
    }
}
