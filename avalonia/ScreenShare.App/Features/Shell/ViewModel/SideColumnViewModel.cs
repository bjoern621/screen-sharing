using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Mvvm;
using TablerIcons;

namespace ScreenShare.App.Features.Shell.ViewModel;

/// <summary>
/// One surface's side column, read against the width the window has.
/// The window owns that width and states it on every change (<see cref="ShellViewModel"/>),
/// so a column wider than the window has room for becomes a panel over the body,
/// with nothing on the surface to update (<c>docs/design-language.md</c>, "Narrow windows").
///
/// Owned here: whether the reader has the panel open.
/// Everything else is derived from the row and the width on the pass that reads it.
/// </summary>
public sealed class SideColumnViewModel : Observable
{
    private readonly SideColumn _column;
    private readonly string _openTip;
    private readonly string _closeTip;

    /// <summary>
    /// Width the window last stated.
    /// Infinite until it states one: an unasked question withholds nothing, so a first paint draws the columns
    /// the surface states and the first measured pass narrows them
    /// (<c>docs/development-principles.md</c>, "Capabilities the machine decides").
    /// </summary>
    private double _window = double.PositiveInfinity;

    private bool _opened;

    public SideColumnViewModel(SideColumn column, string openTip, string closeTip)
    {
        Assert.That(column.Width > 0 && column.Body > 0, "a side column states a width and the body beside it", column.Width, column.Body);
        Assert.That(openTip.Length > 0 && closeTip.Length > 0, "a toggle names both states it leads to", openTip, closeTip);

        _column = column;
        _openTip = openTip;
        _closeTip = closeTip;

        Toggle = new DelegateCommand(() =>
        {
            _opened = !_opened;
            Apply();
        });
    }

    /// <summary>Opens the panel, or closes it again.</summary>
    public DelegateCommand Toggle { get; }

    public double Width => _column.Width;

    public bool IsOnLeft => _column.Edge == ColumnEdge.Left;

    /// <summary>Whether the window carries the column beside the body.</summary>
    public bool IsBeside => _column.FitsBeside(_window);

    /// <summary>
    /// Whether the column is drawn at all.
    /// A column that draws unasked is on screen wherever it fits, and on a narrower window whenever the reader
    /// opened it.
    /// </summary>
    public bool IsShowing => (_column.DrawnUnasked && IsBeside) || _opened;

    /// <summary>
    /// Whether the surface offers the toggle.
    /// A column drawn unasked has nothing to press while it stands beside the body, a press there changing
    /// nothing the reader can see.
    /// </summary>
    public bool ShowsToggle => !_column.DrawnUnasked || !IsBeside;

    public Icons ToggleGlyph => (IsOnLeft, IsShowing) switch
    {
        (true, true) => Icons.IconLayoutSidebarLeftCollapse,
        (true, false) => Icons.IconLayoutSidebarLeftExpand,
        (false, true) => Icons.IconLayoutSidebarRightCollapse,
        (false, false) => Icons.IconLayoutSidebarRightExpand,
    };

    public string ToggleTip => IsShowing ? _closeTip : _openTip;

    /// <summary>
    /// States the width the window has.
    /// Idempotent: the same width twice writes no property and moves nothing.
    /// </summary>
    public void SetWindowWidth(double width)
    {
        Assert.That(width > 0, "a window states a width it has", width);

        _window = width;
        Apply();
    }

    /// <summary>Closes the panel. A panel already closed is the state the caller asked for.</summary>
    public void Close()
    {
        _opened = false;
        Apply();
    }

    /// <summary>
    /// One render function.
    /// Every property here is derived, so the pass raises them all and a binding re-reads what it draws.
    /// </summary>
    public void Apply()
    {
        OnPropertyChanged(nameof(IsBeside));
        OnPropertyChanged(nameof(IsShowing));
        OnPropertyChanged(nameof(ShowsToggle));
        OnPropertyChanged(nameof(ToggleGlyph));
        OnPropertyChanged(nameof(ToggleTip));
    }
}
