using System.Windows.Input;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Mvvm;

/// <summary>
/// A command over an effect that costs a round trip, and the owner of the one fact a
/// <see cref="DelegateCommand"/> cannot hold: whether the call it started is still in flight.
///
/// <b>Why the flag lives here rather than in each view model.</b> Every asked-for effect on this surface has
/// the same three properties - it takes long enough for a reader to wonder, a second press while it runs
/// would ask for the same thing twice, and the answer arrives on whichever thread the transport completed on.
/// Written out per call site those are three chances to forget one; the setup flow's start and its uplink
/// measurement each carried their own copy, and the viewer's two toggles carried none, so a press that had
/// already been sent still looked unpressed.
///
/// <b>It is written in exactly two places</b> - the press and the completion - and read through everywhere
/// else: <see cref="CanExecute"/> refuses a second press off the same field a binding draws the spinner from,
/// so the button a reader sees and the guard that would refuse them cannot disagree.
///
/// The effect keeps its own errors.
/// A refusal is the backend's sentence about an attempt and belongs on the screen that asked
/// (<c>docs/ipc-api.md</c>, "Errors"), so this type catches nothing and only guarantees that the flag clears
/// whichever way the call ended.
/// </summary>
public sealed class PendingCommand : Observable, ICommand
{
    private readonly Func<Task> _run;
    private readonly Func<bool> _canRun;
    private readonly Action<Action> _dispatch;

    private bool _isRunning;

    public event EventHandler? CanExecuteChanged;

    /// <summary>
    /// Raised when the call starts and again when it ends, so an owner whose render pass reads this state -
    /// the setup flow's publish gate is one - draws both edges.
    /// It is news that something moved and carries nothing, in the way every other signal here does.
    /// </summary>
    public event Action? Changed;

    /// <param name="run">The effect. It owns its own failures and answers when the call is over.</param>
    /// <param name="dispatch">
    /// Hands the completion to the UI loop.
    /// Injected rather than reached for, so this type stays free of a toolkit and a test can pass a
    /// synchronous dispatcher.
    /// </param>
    /// <param name="canRun">Everything other than this command's own flight that gates the press.</param>
    public PendingCommand(Func<Task> run, Action<Action> dispatch, Func<bool>? canRun = null)
    {
        Assert.NotNull(run, "a command needs something to run");
        Assert.NotNull(dispatch, "a command needs a UI loop to marshal its completion back to");

        _run = run;
        _dispatch = dispatch;
        _canRun = canRun ?? (static () => true);
    }

    /// <summary>
    /// Whether the call this command started is still in flight.
    /// The view draws its wait from this and nothing else, so a control that says it is working is a call
    /// that is running.
    /// </summary>
    public bool IsRunning { get => _isRunning; private set => Set(ref _isRunning, value); }

    /// <summary>
    /// A press is offered when nothing this command asked for is outstanding and whatever else gates it
    /// holds.
    /// The in-flight half is not the caller's to remember.
    /// </summary>
    public bool CanExecute(object? parameter) => !IsRunning && _canRun();

    public void Execute(object? parameter)
    {
        // The binding is gated on the same answer; this is the guard that holds when a press and a state
        // change race, which the round trip makes a real interval.
        if (!CanExecute(parameter))
        {
            return;
        }

        IsRunning = true;
        Refresh();
        Changed?.Invoke();

        _ = RunAsync();
    }

    /// <summary>Re-asks <c>canRun</c>. Called from the render function, like every other view update.</summary>
    public void Refresh() => CanExecuteChanged?.Invoke(this, EventArgs.Empty);

    private async Task RunAsync()
    {
        try
        {
            await _run().ConfigureAwait(false);
        }
        finally
        {
            // Whichever way the call ended, including one that threw past the effect's own handling: a flag
            // left set is a control that never comes back.
            _dispatch(Finished);
        }
    }

    private void Finished()
    {
        IsRunning = false;
        Refresh();
        Changed?.Invoke();

        Assert.That(!IsRunning, "a command that answered is pressable again");
    }
}
