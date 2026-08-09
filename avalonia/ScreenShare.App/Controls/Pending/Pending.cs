using Avalonia;
using Avalonia.Controls;

namespace ScreenShare.App.Controls;

/// <summary>
/// Says that the control it is set on has asked the backend for something and has not been
/// answered yet. What that looks like is <c>Design/Pending.axaml</c>; this is only the seam
/// between a view model's fact and a style's selector.
///
/// <b>It is an attached property rather than a control</b> because the fact belongs to every
/// control that starts an effect - a button, a toggle, a card - and a wait drawn once per call
/// site is a wait that reads differently in each. Bound to
/// <see cref="Mvvm.PendingCommand.IsRunning"/>, which is the same field the press is refused
/// off, so a control that says it is working is a call that is really in flight.
/// </summary>
public static class Pending
{
    /// <summary>The pseudo-class the design draws the wait from. Set here and named nowhere else.</summary>
    private const string PseudoClass = ":pending";

    public static readonly AttachedProperty<bool> IsActiveProperty =
        AvaloniaProperty.RegisterAttached<Control, bool>("IsActive", typeof(Pending));

    static Pending()
        => IsActiveProperty.Changed.AddClassHandler<Control>(
            (control, change) => ((IPseudoClasses)control.Classes).Set(PseudoClass, change.GetNewValue<bool>()));

    public static bool GetIsActive(Control control) => control.GetValue(IsActiveProperty);

    public static void SetIsActive(Control control, bool value) => control.SetValue(IsActiveProperty, value);
}
