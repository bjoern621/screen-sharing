using Avalonia;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Interactivity;
using Avalonia.Styling;
using Avalonia.VisualTree;
using ScreenShare.App.Features.Shell.Model;

namespace ScreenShare.App.Features.Shell.View;

/// <summary>
/// The app's window.
/// The title bar marks its own caption roles, so the move gesture, the double-click maximise and the window
/// menu are handed to the platform by the markup rather than by a press handler here.
///
/// The one thing that is not markup is <see cref="DropFocus"/>, which is the window's job because focus is
/// the window's: a text box holds the caret and its selection until something takes them away, and on every
/// screen here most of the surface is a panel that takes nothing.
/// So a reader who typed a bitrate, clicked onto the card behind it and pressed a key was still editing the
/// bitrate.
/// </summary>
public sealed partial class ShellWindow : Window
{
    public ShellWindow()
    {
        InitializeComponent();

        // Two halves of one decision, applied together so no platform can end up with two captions: the
        // client area covers the native one, and the replacement the platform then asks for is emptied.
        // Where the desktop draws the frame both stay off (WindowChrome).
        //
        // The third half - the band that stands in for the caption - is bound off the shell instead of
        // written here.
        // Whether it is drawn is now two facts rather than one, since it comes off the window while a stream
        // is filling it, and a property written from here as well would be a second author of one of them
        // (ShellViewModel.HasCaption).
        if (WindowChrome.AppDrawsCaption)
        {
            ExtendClientAreaToDecorationsHint = true;
            WindowDecorationsTheme = (ControlTheme)Resources["EmptyDecorations"]!;
        }

        // Tunnelling, so the press is seen on the way down and whatever it lands on still handles it - a
        // button pressed while a box has the caret takes focus for itself immediately afterwards, which is
        // the outcome either way.
        AddHandler(PointerPressedEvent, DropFocus, RoutingStrategies.Tunnel);
    }

    /// <summary>
    /// Takes this window out of the captures this machine produces, once the platform has given it a handle
    /// to say it about (<c>Features/Shell/Model/CaptureExclusion.cs</c>).
    ///
    /// The whole window rather than the tiles inside it, because the picture a tile draws is a capture of the
    /// screen this window is on: what leaves the window in the capture is what nests a copy of the screen
    /// inside the stream on every round trip.
    /// A system with no way to state it is left as it is.
    /// </summary>
    protected override void OnOpened(EventArgs e)
    {
        base.OnOpened(e);

        CaptureExclusions.ForThisSystem().Exclude(this);
    }

    /// <summary>
    /// Ends a text edit when the press lands anywhere that is not a text box.
    /// Presses inside one are left alone: that is a click into the field the caret is already in, or into a
    /// different one, and the box moves focus itself.
    /// </summary>
    private void DropFocus(object? sender, PointerPressedEventArgs e)
    {
        if (e.Source is Visual source && source.GetSelfAndVisualAncestors().Any(visual => visual is TextBox))
        {
            return;
        }

        // Focusing nothing is how the focus manager clears: there is no next element to hand the keyboard to,
        // because the press landed on a panel and a panel takes no input.
        FocusManager?.Focus(null);
    }
}
