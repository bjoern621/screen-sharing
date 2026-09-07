using Avalonia;
using Avalonia.Controls;
using Avalonia.Input.Platform;
using Avalonia.Threading;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Controls;

/// <summary>
/// Puts one value on the clipboard, drawn inside the box holding it.
///
/// A press wears a tick for a moment: a clipboard write shows nothing on screen,
/// and a control answering a press with nothing reads as broken
/// (<c>docs/design-language.md</c>, "Pressing").
/// The tick is a pseudo-class the theme draws, and it clears itself,
/// so nothing here outlives the press (<c>CopyButton.axaml</c>).
/// </summary>
public sealed class CopyButton : Button
{
    private const string PseudoClass = ":copied";

    /// <summary>How long the tick stands after a press.</summary>
    private static readonly TimeSpan Marked = TimeSpan.FromSeconds(1.5);

    public static readonly StyledProperty<string> ValueProperty =
        AvaloniaProperty.Register<CopyButton, string>(nameof(Value), "");

    private readonly DispatcherTimer _clear;

    public CopyButton()
    {
        _clear = new DispatcherTimer { Interval = Marked };
        _clear.Tick += (_, _) => Mark(false);
    }

    /// <summary>What a press writes.</summary>
    public string Value
    {
        get => GetValue(ValueProperty);
        set => SetValue(ValueProperty, value);
    }

    protected override void OnClick()
    {
        base.OnClick();

        var top = Assert.NotNull(TopLevel.GetTopLevel(this), "a pressed control is drawn in a window");

        // A platform serving no clipboard leaves the tick off, so nothing claims the value was carried.
        if (top.Clipboard is null)
        {
            return;
        }

        _ = WriteAsync(top.Clipboard, Value);
    }

    /// <summary>The tick follows the write, standing for a value the clipboard took.</summary>
    private async Task WriteAsync(IClipboard clipboard, string value)
    {
        await clipboard.SetTextAsync(value);
        Mark(true);
    }

    /// <summary>
    /// Named states rather than a toggle: a second press asks for the tick that already stands,
    /// and gets it for its own moment.
    /// </summary>
    private void Mark(bool copied)
    {
        ((IPseudoClasses)Classes).Set(PseudoClass, copied);

        _clear.Stop();

        if (copied)
        {
            _clear.Start();
        }
    }
}
