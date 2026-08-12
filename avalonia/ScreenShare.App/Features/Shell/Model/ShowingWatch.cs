using Avalonia.Controls;
using Avalonia.Threading;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// Whether a control is being looked at: it stands in a visual tree, in a window that is in
/// front of the reader.
///
/// <b>Two controls need this fact and it is one fact, so it is written once.</b> The broadcast
/// screen's preview card and the wizard's screen picker both draw pictures the backend has to
/// produce - GPU copies into lent slots, and in the picker's case a screen capture per monitor.
/// Neither is free, and the shell renders every destination on every pass, so a card that drew
/// whenever anything was available would charge a reader who never opened the screen and one
/// that drew for as long as the screen was open would charge a reader whose window has been
/// behind a terminal for an hour.
///
/// <b>Neither half is a fact a view model can read.</b> Whether a control is in a visual tree is
/// the control's own, and whether the window is in front is the platform's
/// (<see cref="IWindowPresence"/>). So a control owns one of these and reports what it says.
///
/// Coming forward is immediate and leaving is delayed, and the asymmetry is deliberate: a reader
/// who raised the window is looking now, and a reader who clicked into a terminal for a moment
/// should not pay for a teardown and a fresh start each time.
/// </summary>
internal sealed class ShowingWatch : IDisposable
{
    /// <summary>
    /// How long the window has to stay out of the foreground before the answer turns false.
    ///
    /// Leaving costs whatever the consumer had open: a frame subscription closes and the pool
    /// behind it is freed, and coming back re-announces the pool and imports it again. A
    /// notification that takes focus and gives it back would otherwise pay for that round trip
    /// every time.
    /// </summary>
    private static readonly TimeSpan LeaveDelay = TimeSpan.FromSeconds(1);

    private readonly Control _control;

    /// <summary>Told whenever the answer moves, and told the answer rather than asked for it.</summary>
    private readonly Action<bool> _report;

    /// <summary>Whether the control is in a visual tree, which is half of the answer.</summary>
    private bool _attached;

    /// <summary>
    /// The window's own answer to whether it is in front, held while the control is attached and
    /// dropped with the tree. It is the reader rather than a copy of what it said, so the state
    /// stays the window's.
    /// </summary>
    private IWindowPresence? _presence;

    /// <summary>The timer that ends the wait after the window has stayed away.</summary>
    private DispatcherTimer? _leaving;

    public ShowingWatch(Control control, Action<bool> report)
    {
        Assert.NotNull(control, "a showing watch belongs to a control");
        Assert.NotNull(report, "a showing watch reports what it found to somebody");

        _control = control;
        _report = report;
    }

    /// <summary>Whether the control is being looked at, as of now.</summary>
    public bool Showing => _attached && _presence is { IsInFront: true };

    /// <summary>
    /// The control entered a visual tree. It finds the window it is in from the control itself,
    /// because that is the only side that knows where it was put.
    /// </summary>
    public void Attached()
    {
        _attached = true;
        Watch(Assert.NotNull(TopLevel.GetTopLevel(_control) as Window, "a watched control is drawn in a window"));
        Report();
    }

    /// <summary>The control left the visual tree. There is nothing left to draw in and nothing to wait for.</summary>
    public void Detached()
    {
        _attached = false;
        Watch(null);
        Report();
    }

    /// <summary>
    /// Says the answer again without anything having moved. It exists for the one caller that
    /// has news this class cannot see: a control handed a different view model has to report to
    /// the new one, and the answer is the same.
    /// </summary>
    public void Repeat() => Report();

    public void Dispose()
    {
        _leaving?.Stop();
        _leaving = null;
        Watch(null);
    }

    /// <summary>
    /// Reads presence off one window, or off none. Idempotent, and the only writer of
    /// <see cref="_presence"/>: whatever was being read is dropped first, so a control moved
    /// between windows holds one subscription rather than two.
    /// </summary>
    private void Watch(Window? window)
    {
        if (_presence is not null)
        {
            _presence.Changed -= Report;
            _presence.Dispose();
            _presence = null;
        }

        if (window is null)
        {
            return;
        }

        _presence = WindowPresences.For(window);
        _presence.Changed += Report;
    }

    /// <summary>
    /// Reports the answer, immediately where it is certain and after the wait where it is not.
    ///
    /// Both certain answers are given at once: the control is being looked at, so whatever it
    /// draws is wanted now, or it has left the tree, so there is nothing left to draw it with
    /// and nothing to wait for.
    /// </summary>
    private void Report()
    {
        if (Showing || !_attached)
        {
            _leaving?.Stop();
            _report(Showing);
            return;
        }

        Wait();
    }

    /// <summary>
    /// Starts the wait, and lets one already running finish rather than restarting it: the
    /// window going further behind is not a reason to keep drawing longer.
    /// </summary>
    private void Wait()
    {
        _leaving ??= Leaving();

        if (!_leaving.IsEnabled)
        {
            _leaving.Start();
        }
    }

    private DispatcherTimer Leaving()
    {
        var timer = new DispatcherTimer { Interval = LeaveDelay };
        timer.Tick += (_, _) => Left();
        return timer;
    }

    /// <summary>
    /// Reports the end of the wait. The fact is read again rather than assumed: a window raised
    /// while the timer was in flight is a control that goes on drawing.
    /// </summary>
    private void Left()
    {
        _leaving?.Stop();

        if (!Showing)
        {
            _report(false);
        }
    }
}
