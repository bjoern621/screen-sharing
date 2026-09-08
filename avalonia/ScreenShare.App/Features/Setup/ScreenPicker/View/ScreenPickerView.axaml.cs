using Avalonia.Controls;

namespace ScreenShare.App.Features.Setup.ScreenPicker.View;

/// <summary>
/// Markup.
///
/// Whether the pictures are being looked at is reported by the surface drawing the whole share question,
/// which is where the answer is one fact rather than one per control
/// (<c>Features/Setup/ShareStep/View/ShareStepView.axaml.cs</c>).
/// </summary>
public sealed partial class ScreenPickerView : UserControl
{
    public ScreenPickerView() => InitializeComponent();
}
