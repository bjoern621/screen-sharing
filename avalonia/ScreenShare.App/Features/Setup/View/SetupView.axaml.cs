using Avalonia.Controls;

namespace ScreenShare.App.Features.Setup.View;

/// <summary>
/// Setup flow as the shell embeds it: a <see cref="UserControl"/> over <c>SetupViewModel</c>, owning no window,
/// title bar or nav strip.
/// Markup and nothing else, so every write is a command or a bound input and the view model's one render function
/// still restores a correct view.
/// </summary>
public sealed partial class SetupView : UserControl
{
    public SetupView() => InitializeComponent();
}
