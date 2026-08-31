using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.ScreenPicker.Model;

/// <summary>
/// One screen the wizard offers, as a row of facts: which output it is, what it is called, what it is showing,
/// and whether it can be picked.
///
/// A record, the tile and the command handed in rather than made here,
/// so two passes over one unchanged screen compare equal.
/// A row that rebuilt either would replace the picture every time the form resolved.
/// </summary>
/// <param name="Monitor">
/// Index the output is enumerated under, what <c>publish.monitor</c> carries and what a preview is opened by.
/// </param>
/// <param name="Label">
/// What the screen is called, written by this shell out of the catalog: <c>Screen 1 · 2560 × 1440 · 144 Hz · main</c>,
/// without the rate where the output reports none.
/// </param>
/// <param name="IsSelected">Whether this is the screen the stream would capture.</param>
/// <param name="IsEnabled">Whether it can be picked, which the form decides.</param>
/// <param name="Reason">Why it cannot. Empty while it can.</param>
/// <param name="Tile">
/// The picture, null until the backend reports that it is reading this screen.
/// Made no earlier: a tile subscribes to frames when it is drawn,
/// and a subscription naming a screen nothing is reading is refused once and never retried,
/// so a tile built ahead of the preview stays dark for as long as the step is open.
/// </param>
/// <param name="Placeholder">Why there is no picture. Empty while there is one.</param>
/// <param name="Select">Picks this screen.</param>
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
    public bool HasReason => Reason.Length > 0;

    public bool HasTile => Tile is not null;

    public bool HasPlaceholder => Placeholder.Length > 0;
}
