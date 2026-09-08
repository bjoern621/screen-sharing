using Avalonia.Controls;
using Avalonia.Input;
using ScreenShare.App.Features.Shell.Consent.ViewModel;

namespace ScreenShare.App.Features.Shell.Consent.View;

/// <summary>
/// Two gestures the markup cannot state, both of them the answer.
/// The toggle the question is answered on writes through its own binding.
/// </summary>
public sealed partial class ConsentView : UserControl
{
    public ConsentView()
    {
        InitializeComponent();

        // Escape is bound here and on the window, which uses it to leave a filled viewer.
        // A key event starts at the focus and travels outward, so the focus moving here as the question opens
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
    /// Press on the dimmed ground, which answers and closes.
    /// The press has to have landed on the ground itself:
    /// one inside the dialog reaches this handler on its way out,
    /// and answering on it would close the question whenever anything in it was clicked.
    /// </summary>
    private void ScrimPressed(object? sender, PointerPressedEventArgs e)
    {
        if (ReferenceEquals(e.Source, sender) && DataContext is ConsentViewModel consent)
        {
            consent.CloseCommand.Execute(null);
        }
    }
}
