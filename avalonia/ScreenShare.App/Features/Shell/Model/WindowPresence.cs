using Avalonia.Controls;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// Whether a window is in front of the reader, for the screens whose cost runs on while nobody is looking.
/// A drawn frame costs a GPU copy into a lent slot, a frame-channel message and a draw, and the wizard's screen
/// picker costs a capture per monitor on top
/// (<c>Features/Setup/ScreenPicker/View/ScreenPickerView.axaml.cs</c>).
/// Answers the question and governs nothing.
///
/// <b>Why an interface.</b> Each windowing system states presence its own way, and better than the toolkit
/// does: <c>NSWindowOcclusionState</c> on macOS, a DWM-cloaked window on Windows, <c>_NET_WM_STATE_HIDDEN</c>
/// on X11, a Wayland compositor withholding frame callbacks.
/// None of it reaches an Avalonia property, so each is a reader of its own.
///
/// <b>Read through, never remembered.</b> <see cref="IsInFront"/> is asked of the window on every pass, and
/// <see cref="Changed"/> carries no answer with it.
/// </summary>
internal interface IWindowPresence : IDisposable
{
    bool IsInFront { get; }

    /// <summary>Raised where the answer moved. A window change that leaves it standing raises nothing.</summary>
    event Action? Changed;
}

/// <summary>
/// One place a platform's own notion of presence is picked, so a screen wanting the fact names no system and no
/// toolkit property.
/// </summary>
internal static class WindowPresences
{
    /// <summary>
    /// One reader per caller, disposed by the caller: it holds a subscription to the window.
    /// <see cref="ToplevelPresence"/> answers every platform, on the facts Avalonia normalises across them.
    /// A system with a better answer of its own is a second implementation and a branch here, with nothing
    /// that reads presence changing.
    /// </summary>
    public static IWindowPresence For(Window window) => new ToplevelPresence(window);
}
