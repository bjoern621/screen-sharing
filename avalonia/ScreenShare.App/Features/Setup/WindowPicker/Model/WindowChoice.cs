using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.WindowPicker.Model;

/// <summary>
/// One window the picker offers, as a row of facts: which handle it is, what it is called,
/// how big it is, and whether it can be picked.
///
/// A record, the command handed in rather than made here,
/// so two passes over one unchanged window compare equal and the bound collection is left alone.
/// </summary>
/// <param name="Handle">
/// What the platform addresses the window by, what <c>publish.share_window</c> carries.
/// </param>
/// <param name="Label">
/// What the window is called, written by this shell out of the list it read: <c>Build log · code</c>.
/// </param>
/// <param name="Size">
/// The captured area, <c>1200 × 900</c>. Empty for a handle the read did not name,
/// a window that closed between the resolve and the read being the case.
/// </param>
/// <param name="IsSelected">Whether this is the window the stream would capture.</param>
/// <param name="IsEnabled">Whether it can be picked, which the form decides.</param>
/// <param name="Reason">Why it cannot. Empty while it can.</param>
/// <param name="Select">Picks this window.</param>
public sealed record WindowChoice(
    string Handle,
    string Label,
    string Size,
    bool IsSelected,
    bool IsEnabled,
    string Reason,
    DelegateCommand Select)
{
    public bool HasSize => Size.Length > 0;

    public bool HasReason => Reason.Length > 0;
}
