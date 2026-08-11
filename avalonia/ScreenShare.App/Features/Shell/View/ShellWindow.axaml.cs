using Avalonia;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Interactivity;
using Avalonia.Styling;
using Avalonia.VisualTree;
using ScreenShare.App.Features.Shell.Model;

namespace ScreenShare.App.Features.Shell.View;

/// <summary>
/// The app's window. The title bar marks its own caption roles, so the move gesture, the
/// double-click maximise and the window menu are handed to the platform by the markup rather
/// than by a press handler here.
///
/// The one thing that is not markup is <see cref="DropFocus"/>, which is the window's job
/// because focus is the window's: a text box holds the caret and its selection until something
/// takes them away, and on every screen here most of the surface is a panel that takes nothing.
/// So a reader who typed a bitrate, clicked onto the card behind it and pressed a key was still
/// editing the bitrate.
/// </summary>
public sealed partial class ShellWindow : Window
{
    public ShellWindow()
    {
        InitializeComponent();

        // The three halves of one decision, applied together so no platform can end up with
        // two captions or with none: the client area covers the native one, the replacement
        // the platform then asks for is emptied, and the band that stands in for it is drawn.
        // Where the desktop draws the frame all three stay off (WindowChrome).
        if (WindowChrome.AppDrawsCaption)
        {
            ExtendClientAreaToDecorationsHint = true;
            WindowDecorationsTheme = (ControlTheme)Resources["EmptyDecorations"]!;
        }

        Caption.IsVisible = WindowChrome.AppDrawsCaption;

        // Tunnelling, so the press is seen on the way down and whatever it lands on still
        // handles it - a button pressed while a box has the caret takes focus for itself
        // immediately afterwards, which is the outcome either way.
        AddHandler(PointerPressedEvent, DropFocus, RoutingStrategies.Tunnel);
    }

    /// <summary>
    /// Ends a text edit when the press lands anywhere that is not a text box. Presses inside
    /// one are left alone: that is a click into the field the caret is already in, or into a
    /// different one, and the box moves focus itself.
    /// </summary>
    private void DropFocus(object? sender, PointerPressedEventArgs e)
    {
        if (e.Source is Visual source && source.GetSelfAndVisualAncestors().Any(visual => visual is TextBox))
        {
            return;
        }

        // Focusing nothing is how the focus manager clears: there is no next element to hand
        // the keyboard to, because the press landed on a panel and a panel takes no input.
        FocusManager?.Focus(null);
    }
}
