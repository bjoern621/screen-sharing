using System.Windows.Input;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Mvvm;

/// <summary>
/// <see cref="ICommand"/> over a delegate,
/// so a button binds to a view-model method rather than to a code-behind handler reaching into widgets.
/// </summary>
public sealed class DelegateCommand : ICommand
{
    private readonly Action _run;
    private readonly Func<bool> _canRun;

    public event EventHandler? CanExecuteChanged;

    public DelegateCommand(Action run, Func<bool>? canRun = null)
    {
        Assert.NotNull(run, "a command needs something to run");

        _run = run;
        _canRun = canRun ?? (static () => true);
    }

    public bool CanExecute(object? parameter) => _canRun();

    public void Execute(object? parameter) => _run();

    /// <summary>Re-asks <c>canRun</c>, from the render function like every other write to the view.</summary>
    public void Refresh() => CanExecuteChanged?.Invoke(this, EventArgs.Empty);
}
