using Avalonia.Controls;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// Whether a window is in front of the reader, for the screens that stop working when it is not.
///
/// <b>What it is for.</b> A picture nobody is looking at still costs what a picture costs - a GPU copy per
/// frame into a lent slot, a message on the frame channel and a draw - so a surface that draws frames reads
/// this and stops while the window is behind something
/// (<c>Features/Broadcast/Preview/View/PreviewView.axaml.cs</c>).
/// It answers one question and governs nothing: what a reader does with the answer is the reader's.
///
/// <b>Why it is an interface.</b> "In front" is a fact each windowing system answers differently, and better
/// than the toolkit does.
/// macOS reports occlusion outright (<c>NSWindowOcclusionState</c>), Windows reports a cloaked window through
/// DWM, X11 has <c>_NET_WM_STATE_HIDDEN</c> and a Wayland compositor stops sending frame callbacks to a
/// surface it is not showing.
/// None of that reaches an Avalonia property, so each is a reader of its own once it is written, and the seam
/// is here rather than in the card that reads it.
///
/// <b>It is read through, never remembered.</b> <see cref="IsInFront"/> is a question asked of the window on
/// every pass; <see cref="Changed"/> only says the answer moved, and carries no answer with it.
/// </summary>
internal interface IWindowPresence : IDisposable
{
    /// <summary>Whether the window is in front of the reader.</summary>
    bool IsInFront { get; }

    /// <summary>
    /// Raised when the answer moved.
    /// A change that leaves it where it was raises nothing, so a listener is not woken to re-derive what it
    /// already drew.
    /// </summary>
    event Action? Changed;
}

/// <summary>
/// Which reader answers for a window.
/// It is the one place a platform's own notion of presence is chosen, so a screen that wants the fact names
/// no system and no toolkit property.
/// </summary>
internal static class WindowPresences
{
    /// <summary>
    /// The presence reader for a window.
    /// One reader per caller, and the caller disposes it: it holds a subscription to the window.
    ///
    /// Every platform is answered by <see cref="ToplevelPresence"/> here, because the facts it reads are the
    /// ones Avalonia normalises across all of them.
    /// A system whose own answer is better than the toolkit's is a second implementation and a branch in this
    /// method, and nothing that reads presence changes.
    /// </summary>
    public static IWindowPresence For(Window window) => new ToplevelPresence(window);
}
