using System.Windows.Input;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Mvvm;

/// <summary>
/// Command over an effect that costs a round trip, holding the fact a <see cref="DelegateCommand"/> cannot:
/// whether the call it started is still out.
///
/// <b>The flag lives here rather than in each view model.</b> Every asked-for effect on this surface waits long
/// enough for a reader to wonder, asks for the same thing twice when a second press lands mid-flight,
/// and answers on whichever thread the transport completed on.
/// Restated per call site, each of those is a chance to leave one out.
///
/// <b>Written on the press and on the completion, read through everywhere else.</b>
/// <see cref="CanExecute"/> refuses the second press off the same field the wait is drawn from,
/// so the control a reader sees and the guard that would refuse them cannot disagree.
///
/// Errors stay with the effect.
/// A refusal is the backend's sentence about an attempt and belongs on the screen that asked
/// (<c>docs/ipc-api.md</c>, "Errors"), so nothing is caught here
/// and the only guarantee is a flag that clears whichever way the call ended.
/// </summary>
public sealed class PendingCommand : Observable, ICommand
{
    private readonly Func<Task> _run;
    private readonly Func<bool> _canRun;
    private readonly Action<Action> _dispatch;

    private bool _isRunning;

    public event EventHandler? CanExecuteChanged;

    /// <summary>
    /// Raised as the call starts and again as it ends, so an owner rendering from this state draws both edges.
    /// Carries nothing, as every signal here does: what moved is read through.
    /// </summary>
    public event Action? Changed;

    /// <param name="run">Effect. Owns its failures, and answers once the call is over.</param>
    /// <param name="dispatch">
    /// Hands the completion to the UI loop.
    /// Injected rather than reached for, so no toolkit is bound in here and a test passes a synchronous
    /// dispatcher.
    /// </param>
    /// <param name="canRun">Everything but this command's own flight that gates the press.</param>
    public PendingCommand(Func<Task> run, Action<Action> dispatch, Func<bool>? canRun = null)
    {
        Assert.NotNull(run, "a command needs something to run");
        Assert.NotNull(dispatch, "a command needs a UI loop to marshal its completion back to");

        _run = run;
        _dispatch = dispatch;
        _canRun = canRun ?? (static () => true);
    }

    /// <summary>
    /// Whether the call this command started is still out.
    /// The view draws its wait from this and from nothing else, so a control drawing one is a call in flight.
    /// </summary>
    public bool IsRunning { get => _isRunning; private set => Set(ref _isRunning, value); }

    /// <summary>
    /// Offered while nothing this command asked for is outstanding and whatever else gates it holds.
    /// Both halves are asked per press rather than read off a verdict the last render composed,
    /// and the in-flight one is not the caller's to remember.
    /// </summary>
    public bool CanExecute(object? parameter) => !IsRunning && _canRun();

    public void Execute(object? parameter)
    {
        // Asked at the press rather than trusted from the render the binding was drawn on: a press and a state
        // change race over an interval the round trip makes real.
        if (!CanExecute(parameter))
        {
            return;
        }

        IsRunning = true;
        Refresh();
        Changed?.Invoke();

        _ = RunAsync();
    }

    /// <summary>Re-asks <c>canRun</c>, from the render function like every other write to the view.</summary>
    public void Refresh() => CanExecuteChanged?.Invoke(this, EventArgs.Empty);

    private async Task RunAsync()
    {
        try
        {
            await _run().ConfigureAwait(false);
        }
        finally
        {
            // Cleared whichever way the call ended, a throw past the effect's own handling included: a flag
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
