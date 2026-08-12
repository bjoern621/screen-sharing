using Avalonia;
using Avalonia.Controls;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// Presence as the toolkit already reports it, which is the reading every platform gets until
/// one of them is given its own.
///
/// <b>The three facts.</b> A window is in front while it is visible, not minimised and active.
/// Active is the one that carries the answer on every system here: a window behind another,
/// and on a compositor with workspaces a window on one nobody is looking at, is not the window
/// holding activation. The other two are separate states rather than consequences of it -
/// a hidden window and a minimised one are not "behind something", and a platform is free to
/// keep either active.
///
/// <b>What it cannot see.</b> A window that is fully covered by another while it still holds
/// activation, and a window scrolled off a virtual desktop that keeps its toplevel activated,
/// both read as in front. Both are the platform's answer to give
/// (<see cref="WindowPresences.For"/>), and neither is guessed at here.
/// </summary>
internal sealed class ToplevelPresence : IWindowPresence
{
    private readonly Window _window;

    /// <summary>
    /// What was last announced, so a window property that moved without moving the answer
    /// raises nothing.
    /// </summary>
    private bool _announced;

    public ToplevelPresence(Window window)
    {
        Assert.NotNull(window, "a presence reads a window");

        _window = window;
        _announced = IsInFront;
        _window.PropertyChanged += OnWindowChanged;
    }

    public event Action? Changed;

    public bool IsInFront
        => _window.IsVisible
            && _window.WindowState != WindowState.Minimized
            && _window.IsActive;

    public void Dispose() => _window.PropertyChanged -= OnWindowChanged;

    /// <summary>
    /// Watches the three properties the answer is derived from, and re-derives it rather than
    /// reading a state out of the change: the properties move one at a time and only their
    /// conjunction is the fact.
    /// </summary>
    private void OnWindowChanged(object? sender, AvaloniaPropertyChangedEventArgs e)
    {
        if (e.Property != Window.IsActiveProperty
            && e.Property != Window.WindowStateProperty
            && e.Property != Visual.IsVisibleProperty)
        {
            return;
        }

        var front = IsInFront;
        if (front == _announced)
        {
            return;
        }

        _announced = front;
        Changed?.Invoke();
    }
}
