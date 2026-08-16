using Avalonia;
using Avalonia.Controls;

namespace ScreenShare.App.Controls;

/// <summary>
/// Says the control it is set on has asked the backend for something and has not been answered.
/// Attached rather than a control of its own, the fact belonging to every kind that starts an effect, and what a
/// wait looks like being stated once in <c>Design/Pending.axaml</c>.
/// Bound to <see cref="Mvvm.PendingCommand.IsRunning"/>, the same field the press is refused off, so a control
/// that looks busy has a call in flight.
/// </summary>
public static class Pending
{
    /// <summary>Pseudo-class <c>Design/Pending.axaml</c> selects the wait on.</summary>
    private const string PseudoClass = ":pending";

    public static readonly AttachedProperty<bool> IsActiveProperty =
        AvaloniaProperty.RegisterAttached<Control, bool>("IsActive", typeof(Pending));

    static Pending()
        => IsActiveProperty.Changed.AddClassHandler<Control>(
            (control, change) => ((IPseudoClasses)control.Classes).Set(PseudoClass, change.GetNewValue<bool>()));

    public static bool GetIsActive(Control control) => control.GetValue(IsActiveProperty);

    public static void SetIsActive(Control control, bool value) => control.SetValue(IsActiveProperty, value);
}
