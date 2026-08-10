using Avalonia.Controls;

namespace ScreenShare.App.Features.Fields.View;

/// <summary>
/// Markup and nothing else. Every control on this screen writes the draft through its
/// binding, and the next resolved form decides what the write became.
/// </summary>
public sealed partial class FieldGroupView : UserControl
{
    public FieldGroupView() => InitializeComponent();
}
