using Avalonia;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Markup.Xaml;

using ScreenShare.App.Features.Viewer.Tile.ViewModel;

namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// One tile, wherever it is drawn.
///
/// The code-behind is two things the markup cannot state: the meter's bar is a width in pixels
/// rather than a proportion any panel offers, and the volume slider inside the menu has to keep
/// the menu open while it is dragged.
/// </summary>
public partial class TileCard : UserControl
{
    public TileCard()
    {
        InitializeComponent();

        // The bar is sized from the level and from this control's own width, so both are watched.
        // It is done here rather than with a converter because a converter has one input and this
        // needs two.
        PropertyChanged += (_, change) =>
        {
            if (change.Property == BoundsProperty || change.Property == DataContextProperty)
            {
                Draw();
            }
        };

        DataContextChanged += (_, _) => Watch();
    }

    private TileViewModel? _watched;

    /// <summary>
    /// Follows the view model's notifications so the meter redraws when a level lands.
    ///
    /// The level arrives on its own path at fifteen a second and moves two properties; this
    /// redraws one border's width from them and touches nothing else on the screen
    /// (<c>Backend/Session.cs</c>, <c>Metered</c>).
    /// </summary>
    private void Watch()
    {
        if (_watched is not null)
        {
            _watched.PropertyChanged -= OnTileChanged;
        }

        _watched = DataContext as TileViewModel;
        if (_watched is not null)
        {
            _watched.PropertyChanged += OnTileChanged;
        }

        Draw();
    }

    private void OnTileChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs change)
    {
        if (change.PropertyName is nameof(TileViewModel.Level) or nameof(TileViewModel.HasLevel))
        {
            Draw();
        }
    }

    /// <summary>Sizes the meter's bar to the level it is showing.</summary>
    private void Draw()
    {
        if (this.FindControl<Border>("Level") is not { } bar)
        {
            return;
        }

        bar.Width = _watched is { HasLevel: true } tile ? Math.Max(0, Bounds.Width * tile.Level) : 0;
    }

    /// <summary>
    /// Lets the volume slider be dragged inside the menu.
    ///
    /// A press inside a menu item is a click on that item, and a click closes the flyout - so
    /// without this the slider would take one press and the drag would land on a menu that was
    /// already gone. Marking the pointer events handled stops them reaching the item;
    /// <c>StaysOpenOnClick</c> in the markup is the other half, for the press that does get
    /// through.
    /// </summary>
    private void OnVolumePressed(object? sender, PointerPressedEventArgs e) => e.Handled = true;

    private void InitializeComponent() => AvaloniaXamlLoader.Load(this);
}
