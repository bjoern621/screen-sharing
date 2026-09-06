using Avalonia.Controls;
using Avalonia.Input;
using ScreenShare.App.Features.Shell.Settings.ViewModel;

namespace ScreenShare.App.Features.Shell.Settings.View;

/// <summary>
/// Two gestures the markup cannot state, both of them dismissals.
/// Every setting the dialog draws writes through its own binding.
/// </summary>
public sealed partial class AppSettingsView : UserControl
{
    public AppSettingsView()
    {
        InitializeComponent();

        // Escape is bound here and on the window, which uses it to leave a filled viewer.
        // A key event starts at the focus and travels outward, so the focus moving here as the dialog opens
        // is what puts this binding first.
        PropertyChanged += (_, change) =>
        {
            if (change.Property == IsVisibleProperty && IsVisible)
            {
                Focus();
            }
        };
    }

    /// <summary>
    /// Press on the dimmed ground, which closes.
    /// The press has to have landed on the ground itself:
    /// one inside the dialog reaches this handler on its way out,
    /// and closing on it would dismiss the dialog whenever anything in it was clicked.
    /// </summary>
    private void ScrimPressed(object? sender, PointerPressedEventArgs e)
    {
        if (ReferenceEquals(e.Source, sender) && DataContext is AppSettingsViewModel settings)
        {
            settings.CloseCommand.Execute(null);
        }
    }
}
