using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.ScreenPicker.Model;

/// <summary>
/// One screen the wizard offers, as a row of facts: which output it is, what it is called, what it is showing
/// right now, and whether it can be picked.
///
/// <b>A record, so a pass that changed nothing changes nothing on screen.</b> The tile and the command are
/// held across passes and handed in rather than made here, which is what lets two passes over one unchanged
/// screen compare equal - a row that rebuilt either would replace the picture every time the form resolved.
/// </summary>
/// <param name="Monitor">The index the output is enumerated under, which is what <c>publish.monitor</c>
/// carries and what the preview is opened by.</param> <param name="Label">What the screen is called: its
/// size, its refresh rate and whether it is the main one.
/// Written by this shell out of the catalog, like every other name.</param> <param name="IsSelected">Whether
/// this is the screen the stream would capture.</param> <param name="IsEnabled">Whether it can be picked,
/// which the form decides.</param> <param name="Reason">Why it cannot, empty while it can.</param>
/// <param name="Tile">The picture, and null until the backend reports that it is reading this screen.
/// It is made no earlier on purpose: a tile subscribes to frames when it is drawn and a subscription that
/// names a screen nothing is reading is refused once and never retried, so a tile built ahead of the preview
/// would stay dark for as long as the step is open.</param> <param name="Placeholder">Why there is no
/// picture, empty while there is one.</param> <param name="Select">Picks this screen.</param>
public sealed record ScreenChoice(
    int Monitor,
    string Label,
    bool IsSelected,
    bool IsEnabled,
    string Reason,
    TileViewModel? Tile,
    string Placeholder,
    DelegateCommand Select)
{
    /// <summary>Whether the row has a reason to print, which is the state that draws it greyed.</summary>
    public bool HasReason => Reason.Length > 0;

    /// <summary>Whether there is a picture on the row, which is what separates it from its dark state.</summary>
    public bool HasTile => Tile is not null;

    public bool HasPlaceholder => Placeholder.Length > 0;
}
