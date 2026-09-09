using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.RegionPicker.Model;
using ScreenShare.App.Features.Setup.ShareQuestion.ViewModel;
using ScreenShare.App.Features.Viewer.Tile.Model;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.RegionPicker.ViewModel;

/// <summary>
/// The rectangle a stream is cropped to: what one reads back as, a live picture of it, and the press that draws
/// another.
///
/// A rectangle is dragged over the desktop rather than typed, so the chooser replaces the text control
/// the form offers and the drag is the only way in.
/// Nothing becomes unreachable by it: every rectangle a reader can name is one they can drag.
///
/// <b>The picture is the screen's, cropped.</b>
/// A preview reads one whole output,
/// so the rectangle is drawn by scaling that output's picture until the rectangle fills the box
/// (<see cref="MonitorPreviews"/>).
/// The numbers below are that arithmetic, and the view arranges from them.
///
/// <b>One screen holds the rectangle or nothing draws it.</b>
/// Which screen a capture reads is the backend's answer, and this picks the screen whose own rectangle holds
/// the drawn one so it knows which preview to want.
/// A rectangle across two screens leaves that unanswered, and a crop of the wrong one would be a picture
/// nobody drew.
/// </summary>
public sealed class RegionPickerViewModel : Observable
{
    private readonly Session _session;
    private readonly MonitorPreviews _previews;

    /// <summary>
    /// Draws a rectangle over the desktop and answers what was drawn, empty where the reader dropped it.
    /// Injected because a rectangle is drawn in a window over the desktop, which a view model does not open
    /// (<c>avalonia/README.md</c>).
    /// </summary>
    private readonly Func<Task<string>> _draw;

    /// <summary>Writes the drawn rectangle into the form session's draft.</summary>
    private readonly Action<string> _write;

    /// <summary>Question this chooser sits in is on screen. Written by the question drawing it.</summary>
    private bool _drawn;

    public RegionPickerViewModel(
        Session session,
        MonitorPreviews previews,
        Func<Task<string>> draw,
        Action<Action> dispatch,
        Action<string> write)
    {
        Assert.NotNull(session, "a region chooser measures the rectangle against this machine's outputs");
        Assert.NotNull(previews, "a region chooser draws the question's own previews");
        Assert.NotNull(draw, "a region chooser needs somewhere to draw a rectangle");
        Assert.NotNull(dispatch, "a region chooser marshals a drawn rectangle back to the UI loop");
        Assert.NotNull(write, "a region chooser writes the rectangle it was handed");

        _session = session;
        _previews = previews;
        _draw = draw;
        _write = write;

        DrawCommand = new PendingCommand(DrawAsync, dispatch, () => IsVisible);
    }

    // --- Outputs ------------------------------------------------------------------

    private bool _isVisible;
    private bool _hasRegion;
    private string _region = RegionCopy.Nothing;
    private string _label = RegionCopy.Draw;
    private bool _hasPreview;
    private TileViewModel? _tile;
    private string _placeholder = "";
    private string _notice = "";
    private int _sourceWidth;
    private int _sourceHeight;
    private int _cropX;
    private int _cropY;
    private int _cropWidth;
    private int _cropHeight;

    /// <summary>
    /// Whether the chooser is drawn at all, which is whether the form is offering the control.
    /// The backend draws it under the region kind alone, so this follows the field
    /// rather than reading the kind itself.
    /// </summary>
    public bool IsVisible { get => _isVisible; private set => Set(ref _isVisible, value); }

    /// <summary>The rectangle as it reads back, or what to do when there is none.</summary>
    public string Region { get => _region; private set => Set(ref _region, value); }

    /// <summary>Whether <see cref="Region"/> is a rectangle rather than the empty state.</summary>
    public bool HasRegion { get => _hasRegion; private set => Set(ref _hasRegion, value); }

    /// <summary>What the button says, which follows whether there is a rectangle to replace.</summary>
    public string Label { get => _label; private set => Set(ref _label, value); }

    /// <summary>Screens this chooser is asking the backend to read: the one holding the rectangle, or none.</summary>
    public IReadOnlyList<int> Wanted { get; private set; } = [];

    /// <summary>Whether the box over the button is drawn, which is whether one screen holds the rectangle.</summary>
    public bool HasPreview { get => _hasPreview; private set => Set(ref _hasPreview, value); }

    /// <summary>The screen's picture, null until the backend reports that it is reading that screen.</summary>
    public TileViewModel? Tile { get => _tile; private set => Set(ref _tile, value); }

    public bool HasTile => Tile is not null;

    /// <summary>
    /// Why there is no box, which is a rectangle no single screen holds.
    /// Empty in every other state: nothing drawn yet already reads back as what to do.
    /// </summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice => Notice.Length > 0;

    /// <summary>Why the box is empty. Empty once there is a picture.</summary>
    public string Placeholder { get => _placeholder; private set => Set(ref _placeholder, value); }

    public bool HasPlaceholder => Placeholder.Length > 0;

    /// <summary>The previewed screen's size, which the picture in the box covers exactly.</summary>
    public int SourceWidth { get => _sourceWidth; private set => Set(ref _sourceWidth, value); }

    public int SourceHeight { get => _sourceHeight; private set => Set(ref _sourceHeight, value); }

    /// <summary>The rectangle in that screen's own pixels, the setting counting from the desktop's corner.</summary>
    public int CropX { get => _cropX; private set => Set(ref _cropX, value); }

    public int CropY { get => _cropY; private set => Set(ref _cropY, value); }

    public int CropWidth { get => _cropWidth; private set => Set(ref _cropWidth, value); }

    public int CropHeight { get => _cropHeight; private set => Set(ref _cropHeight, value); }

    public PendingCommand DrawCommand { get; }

    // --- Inputs -------------------------------------------------------------------

    /// <summary>
    /// The region control as the form resolved it, and whether the question it answers is on screen.
    /// The one render function, safe to run twice.
    /// </summary>
    public void Apply(FieldViewModel? region, bool drawn)
    {
        _drawn = drawn;

        // Reachable rather than merely present.
        // A machine whose desktop picks for itself leaves every control here visible and disabled,
        // and a rectangle drawn against one would be a rectangle nothing reads.
        IsVisible = region is not null && region.IsEnabled;

        var text = region?.Text ?? "";
        var read = ShareRegion.TryRead(text, out var x, out var y, out var width, out var height);
        HasRegion = IsVisible && read;
        Region = Readback(text);
        Label = HasRegion ? RegionCopy.Redraw : RegionCopy.Draw;
        DrawCommand.Refresh();

        Preview(HasRegion ? Holding(x, y, width, height) : null, x, y, width, height);

        Assert.That(!HasRegion || IsVisible, "a rectangle reads back only where one is being picked", Region);
        Assert.That(!HasPreview || HasRegion, "a picture is of a rectangle somebody drew", Region);
        Assert.That(Wanted.Count == 0 || HasPreview, "a screen is read only for a picture", Wanted.Count);
    }

    /// <summary>
    /// The picture and the arithmetic under it, written whole on every pass.
    /// A screen the rectangle has left is dropped by the same pass that stops wanting it.
    /// </summary>
    private void Preview(Api.V1.Monitor? screen, int x, int y, int width, int height)
    {
        // The picture costs a screen capture, so it follows the question being on screen the way the grid does.
        if (screen is null || !_drawn)
        {
            Wanted = [];
            HasPreview = false;
            Tile = null;
            Placeholder = "";
            Notice = HasRegion && screen is null ? RegionCopy.NoScreen : "";
            return;
        }

        Wanted = [screen.Index];
        HasPreview = true;
        Notice = "";

        SourceWidth = screen.Width;
        SourceHeight = screen.Height;
        CropX = x - screen.OffsetX;
        CropY = y - screen.OffsetY;
        CropWidth = width;
        CropHeight = height;

        var tile = _previews.TileOf(screen.Index);
        // No sample: a screen is read rather than received, so no decode holds counters about it.
        tile?.Apply(TilePipeline.Of(_previews.Previewed(screen.Index)), sample: null);

        Tile = tile;
        Placeholder = tile is null ? _previews.PlaceholderFor(screen.Index) : "";
    }

    /// <summary>
    /// The screen whose own rectangle holds this one, null where no single screen does.
    /// The first match: outputs of a virtual desktop do not overlap.
    /// </summary>
    private Api.V1.Monitor? Holding(int x, int y, int width, int height)
    {
        foreach (var screen in _session.Monitors)
        {
            if (x >= screen.OffsetX
                && y >= screen.OffsetY
                && x + width <= screen.OffsetX + screen.Width
                && y + height <= screen.OffsetY + screen.Height)
            {
                return screen;
            }
        }

        return null;
    }

    /// <summary>
    /// The rectangle in words, or the empty state.
    /// A value in no spelling this side reads goes back as it stands: it is what the settings carry,
    /// and hiding it would leave the reader nothing to compare against.
    /// </summary>
    private static string Readback(string text)
    {
        if (text.Length == 0)
        {
            return RegionCopy.Nothing;
        }

        return ShareRegion.TryRead(text, out var x, out var y, out var width, out var height)
            ? RegionCopy.Region(x, y, width, height)
            : text;
    }

    /// <summary>
    /// Draws a rectangle over the desktop and writes it into the draft.
    ///
    /// A rectangle that came back empty is a reader who dropped the gesture, and it writes nothing:
    /// the draft keeps whatever it held, so backing out of the overlay leaves the last one standing.
    /// </summary>
    private async Task DrawAsync()
    {
        var region = await _draw().ConfigureAwait(true);
        if (region.Length > 0)
        {
            _write(region);
        }
    }
}
