using Avalonia;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Interactivity;
using Avalonia.Styling;
using Avalonia.VisualTree;
using ScreenShare.App.Features.Shell.Model;

namespace ScreenShare.App.Features.Shell.View;

/// <summary>
/// App's window.
/// The title bar marks its own caption roles, so the move gesture, the double-click maximise and the window menu
/// reach the platform through the markup rather than through a press handler here.
///
/// <see cref="DropFocus"/> is the one thing that is not markup, and it is the window's because focus is: a text
/// box holds the caret and the selection until something takes them, and most of the surface on every screen here
/// is a panel that takes nothing.
/// </summary>
public sealed partial class ShellWindow : Window
{
    public ShellWindow()
    {
        InitializeComponent();

        // Applied together so no platform ends up with two captions: the client area covers the native one, and
        // the replacement the platform then asks for is emptied.
        // Both stay off where the desktop draws the frame (WindowChrome).
        //
        // Whether the band standing in for the caption is drawn is the shell's to write, it being off while a
        // stream fills the window as well (ShellViewModel.HasCaption).
        if (WindowChrome.AppDrawsCaption)
        {
            ExtendClientAreaToDecorationsHint = true;
            WindowDecorationsTheme = (ControlTheme)Resources["EmptyDecorations"]!;
        }

        // Tunnelling, so the press is seen on the way down and whatever it lands on still handles it.
        // A button pressed while a box holds the caret takes focus for itself straight afterwards.
        AddHandler(PointerPressedEvent, DropFocus, RoutingStrategies.Tunnel);
    }

    /// <summary>
    /// Takes this window out of the captures this machine produces.
    /// Stated on open, which is where the platform has a window handle to state it against
    /// (<c>Features/Shell/Model/CaptureExclusion.cs</c>).
    ///
    /// The whole window rather than the tiles in it: a tile draws a decode of the screen the window stands on, so
    /// a window left in the capture nests one more copy of the screen per round trip.
    /// A system with no way to state it is left as it is.
    /// </summary>
    protected override void OnOpened(EventArgs e)
    {
        base.OnOpened(e);

        CaptureExclusions.ForThisSystem().Exclude(this);
    }

    /// <summary>
    /// Ends a text edit where the press lands outside a text box.
    /// A press inside one is left alone, the box moving focus itself.
    /// </summary>
    private void DropFocus(object? sender, PointerPressedEventArgs e)
    {
        if (e.Source is Visual source && source.GetSelfAndVisualAncestors().Any(visual => visual is TextBox))
        {
            return;
        }

        // Focusing nothing is how the focus manager clears: the press landed on a panel, and a panel takes no
        // input, so there is no next element to hand the keyboard to.

        FocusManager?.Focus(null);
    }
}
