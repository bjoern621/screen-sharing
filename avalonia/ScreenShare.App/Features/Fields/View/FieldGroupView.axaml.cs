using Avalonia.Controls;

namespace ScreenShare.App.Features.Fields.View;

/// <summary>
/// Markup only.
/// Every control writes the draft through its binding, and the next resolved form is what the write became.
/// </summary>
public sealed partial class FieldGroupView : UserControl
{
    public FieldGroupView() => InitializeComponent();
}
