using Avalonia.Controls;

namespace ScreenShare.App.Features.Setup.View;

/// <summary>
/// The setup flow as the shell embeds it: a <see cref="UserControl"/> over <c>SetupViewModel</c>, with no
/// window, title bar or nav strip of its own.
/// Markup and nothing else - every write the screen offers is a command or a bound input, so the view model's
/// one render function still restores a correct view by itself.
/// </summary>
public sealed partial class SetupView : UserControl
{
    public SetupView() => InitializeComponent();
}
