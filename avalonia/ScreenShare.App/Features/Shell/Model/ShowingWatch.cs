using Avalonia.Controls;
using Avalonia.Threading;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// Whether a control is being looked at: standing in a visual tree, in a window in front of the reader.
///
/// <b>For a surface whose drawing costs the backend something.</b> The wizard's screen picker is one:
/// its pictures come off a screen capture per monitor the backend opens because the grid asked for it.
/// The shell renders every destination on every pass, so a grid drawing whenever anything is available charges
/// a reader who never opened the screen, and one drawing for as long as the screen is open charges a reader
/// whose window has been behind a terminal for an hour.
///
/// <b>Not the rule for every picture.</b> The insights preview draws what the reader asked it to and follows no
/// window, a publisher's window standing behind the thing being shared for most of a session
/// (<c>Features/Insights/Preview/ViewModel/PreviewViewModel.cs</c>).
/// The test is whether a window going behind means the reader stopped wanting the picture.
///
/// <b>Neither half is a fact a view model can read.</b> Standing in a visual tree is the control's own, and
/// being in front is the platform's (<see cref="IWindowPresence"/>), so a control owns one of these and reports
/// what it says.
///
/// Coming forward is immediate and leaving is delayed: a reader who raised the window is looking now, and one
/// who clicked into a terminal for a moment should not pay a teardown and a fresh start each time.
/// </summary>
internal sealed class ShowingWatch : IDisposable
{
    /// <summary>
    /// Time out of the foreground before the answer turns false.
    ///
    /// Leaving costs what the consumer had open: the frame subscription closes and the pool behind it is freed,
    /// and coming back re-announces that pool and imports it again.
    /// A notification that takes focus and gives it straight back would otherwise pay that round trip each time.
    /// </summary>
    private static readonly TimeSpan LeaveDelay = TimeSpan.FromSeconds(1);

    private readonly Control _control;

    /// <summary>Told when the answer moves, and told the answer rather than asked for it.</summary>
    private readonly Action<bool> _report;

    /// <summary>Half the answer, and the half the control owns.</summary>
    private bool _attached;

    /// <summary>
    /// Window's own answer to being in front, held while the control is attached and dropped with the tree.
    /// The reader itself rather than a copy of what it said, so the state stays the window's.
    /// </summary>
    private IWindowPresence? _presence;

    /// <summary>Ends the wait once the window has stayed away.</summary>
    private DispatcherTimer? _leaving;

    public ShowingWatch(Control control, Action<bool> report)
    {
        Assert.NotNull(control, "a showing watch belongs to a control");
        Assert.NotNull(report, "a showing watch reports what it found to somebody");

        _control = control;
        _report = report;
    }

    /// <summary>Derived on read, never held.</summary>
    public bool Showing => _attached && _presence is { IsInFront: true };

    /// <summary>Window comes from the control, the only side that knows where it was put.</summary>
    public void Attached()
    {
        _attached = true;
        Watch(Assert.NotNull(TopLevel.GetTopLevel(_control) as Window, "a watched control is drawn in a window"));
        Report();
    }

    /// <summary>The control left the visual tree. Nothing to draw in, and nothing to wait for.</summary>
    public void Detached()
    {
        _attached = false;
        Watch(null);
        Report();
    }

    /// <summary>
    /// Says the answer again with nothing having moved.
    /// For the one caller carrying news this class cannot see: a control handed a different view model reports
    /// to the new one.
    /// </summary>
    public void Repeat() => Report();

    public void Dispose()
    {
        _leaving?.Stop();
        _leaving = null;
        Watch(null);
    }

    /// <summary>
    /// Points the presence reading at one window, or at none.
    /// Only writer of <see cref="_presence"/>, and idempotent: what was being read is dropped first, so a control
    /// moved between windows holds one subscription.
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
    /// Reports at once where the answer is certain, after the wait where it is not.
    ///
    /// Certain both ways: a control being looked at wants what it draws now, and one out of the tree has
    /// nothing to draw it with and nothing to wait for.
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
    /// Starts the wait, leaving one already running to finish: a window going further behind is no reason
    /// to draw longer.
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
    /// Ends the wait.
    /// The fact is read again rather than assumed: a window raised while the timer was in flight goes on drawing.
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
