using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.RegionPicker.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.RegionPicker.ViewModel;

/// <summary>
/// The rectangle a stream is cropped to: what one reads back as, and the press that draws another.
///
/// A rectangle is dragged over the desktop rather than typed, so the chooser replaces the text control
/// the form offers and the drag is the only way in.
/// Nothing becomes unreachable by it: every rectangle a reader can name is one they can drag.
/// </summary>
public sealed class RegionPickerViewModel : Observable
{
    /// <summary>
    /// Draws a rectangle over the desktop and answers what was drawn, empty where the reader dropped it.
    /// Injected because a rectangle is drawn in a window over the desktop, which a view model does not open
    /// (<c>avalonia/README.md</c>).
    /// </summary>
    private readonly Func<Task<string>> _draw;

    /// <summary>Writes the drawn rectangle into the form session's draft.</summary>
    private readonly Action<string> _write;

    public RegionPickerViewModel(Func<Task<string>> draw, Action<Action> dispatch, Action<string> write)
    {
        Assert.NotNull(draw, "a region chooser needs somewhere to draw a rectangle");
        Assert.NotNull(dispatch, "a region chooser marshals a drawn rectangle back to the UI loop");
        Assert.NotNull(write, "a region chooser writes the rectangle it was handed");

        _draw = draw;
        _write = write;

        DrawCommand = new PendingCommand(DrawAsync, dispatch, () => IsVisible);
    }

    // --- Outputs ------------------------------------------------------------------

    private bool _isVisible;
    private bool _hasRegion;
    private string _region = RegionCopy.Nothing;
    private string _label = RegionCopy.Draw;

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

    public PendingCommand DrawCommand { get; }

    // --- Inputs -------------------------------------------------------------------

    /// <summary>
    /// The region control as the form resolved it, null where the form is not offering it.
    /// The one render function, safe to run twice.
    /// </summary>
    public void Apply(FieldViewModel? region)
    {
        // Reachable rather than merely present.
        // A machine whose desktop picks for itself leaves every control here visible and disabled,
        // and a rectangle drawn against one would be a rectangle nothing reads.
        IsVisible = region is not null && region.IsEnabled;

        var text = region?.Text ?? "";
        HasRegion = IsVisible && ShareRegion.TryRead(text, out _, out _, out _, out _);
        Region = Readback(text);
        Label = HasRegion ? RegionCopy.Redraw : RegionCopy.Draw;
        DrawCommand.Refresh();

        Assert.That(!HasRegion || IsVisible, "a rectangle reads back only where one is being picked", Region);
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
