using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Interactivity;
using Avalonia.Threading;

using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// Whether the pointer is over a control and has stopped moving.
///
/// <b>For a surface whose picture is the thing being looked at.</b>
/// Hover chrome and the cursor itself stand over the frames, and a pointer parked on a picture is somebody
/// watching rather than aiming, so what the way in raised comes off again until they stir.
///
/// <b>The control's own fact.</b>
/// Pointer movement over it is the only input, and no view model can read it.
///
/// A stir reports at once and a rest waits out the delay: whatever was taken away is back before the hand
/// that reached for it arrives.
/// </summary>
internal sealed class RestingWatch
{
    /// <summary>
    /// Still pointer before the answer turns true.
    /// Long enough to read the name the pointer raised, short enough that settling in front of a stream clears it.
    /// </summary>
    private static readonly TimeSpan RestDelay = TimeSpan.FromSeconds(2);

    /// <summary>
    /// Phases a pointer event is caught in, and caught after whatever it landed on has handled it:
    /// a wheel the stats panel scrolled on is the reader still working the tile.
    /// </summary>
    private const RoutingStrategies Phases = RoutingStrategies.Tunnel | RoutingStrategies.Bubble;

    private readonly Control _control;

    /// <summary>Told the answer rather than asked for it, and told an answer that has not moved.</summary>
    private readonly Action<bool> _report;

    /// <summary>Ends the wait once the pointer has held still.</summary>
    private DispatcherTimer? _resting;

    public RestingWatch(Control control, Action<bool> report)
    {
        Assert.NotNull(control, "a resting watch belongs to a control");
        Assert.NotNull(report, "a resting watch reports what it found to somebody");

        _control = control;
        _report = report;

        // Handlers on the control the watch belongs to, so the watch lives and ends with it.
        control.AddHandler(InputElement.PointerMovedEvent, (_, _) => Stirred(), Phases, handledEventsToo: true);
        control.AddHandler(InputElement.PointerPressedEvent, (_, _) => Stirred(), Phases, handledEventsToo: true);
        control.AddHandler(InputElement.PointerWheelChangedEvent, (_, _) => Stirred(), Phases, handledEventsToo: true);

        // Entering and leaving are direct, so they are taken off the control rather than off a route.
        // A tile arranged under a pointer that never moved enters without a move, and its wait starts there.
        control.PointerEntered += (_, _) => Stirred();
        control.PointerExited += (_, _) => Gone();
    }

    /// <summary>The pointer moved, pressed or scrolled. What was taken away comes back and the wait restarts.</summary>
    private void Stirred()
    {
        _report(false);
        Wait();
    }

    /// <summary>The pointer left. Nothing over this control to take away, and nothing to wait for.</summary>
    private void Gone()
    {
        _resting?.Stop();
        _report(false);
    }

    /// <summary>Restarts the wait, so the delay is time the pointer held still rather than time it was here.</summary>
    private void Wait()
    {
        _resting ??= Resting();
        _resting.Stop();
        _resting.Start();
    }

    private DispatcherTimer Resting()
    {
        var timer = new DispatcherTimer { Interval = RestDelay };
        timer.Tick += (_, _) => Rested();
        return timer;
    }

    /// <summary>
    /// Ends the wait.
    /// The pointer is read again rather than assumed, a control it left in flight hiding nothing.
    /// A flyout open on the control is what the pointer is aiming at, so that wait starts over instead of ending:
    /// a menu the reader is reading through would otherwise lose the cursor picking a row.
    /// </summary>
    private void Rested()
    {
        _resting?.Stop();

        if (_control.ContextFlyout is { IsOpen: true })
        {
            Wait();
            return;
        }

        _report(_control.IsPointerOver);
    }
}
