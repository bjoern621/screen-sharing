using Avalonia;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Interactivity;
using Avalonia.Markup.Xaml;

using ScreenShare.App.Features.Viewer.Tile.ViewModel;

namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// A tile's code-behind, for every host that draws one.
/// Covers what markup cannot state: the meter's bar is a pixel width rather than a proportion any panel offers,
/// the volume slider has to survive the menu's own click handling, and a gesture is not a binding.
/// </summary>
public partial class TileCard : UserControl
{
    public TileCard()
    {
        InitializeComponent();

        // Bar width follows the level and this control's width, so both are watched.
        // A converter takes one input, so the arithmetic sits here.
        PropertyChanged += (_, change) =>
        {
            if (change.Property == BoundsProperty || change.Property == DataContextProperty)
            {
                Draw();
            }
        };

        DataContextChanged += (_, _) => Watch();
    }

    /// <summary>
    /// Whether this stream's picture is in another window, which makes this card draw the plate saying so.
    ///
    /// The host's answer, not the tile's.
    /// A popped-out stream is drawn by two cards off one tile, the grid slot it keeps and the window it moved to,
    /// so a card reading this from the tile would show the plate twice and the picture nowhere.
    ///
    /// The plate subscribes to nothing: the style behind it clears the picture's source, ending the channel,
    /// giving the pool back and stopping a render size being asked for (<see cref="StreamTile"/>).
    /// </summary>
    public static readonly StyledProperty<bool> PictureElsewhereProperty =
        AvaloniaProperty.Register<TileCard, bool>(nameof(PictureElsewhere));

    public bool PictureElsewhere
    {
        get => GetValue(PictureElsewhereProperty);
        set => SetValue(PictureElsewhereProperty, value);
    }

    private TileViewModel? _watched;
    private TopLevel? _root;

    /// <summary>
    /// Subscribes to the tile's keys on the window this card sits in.
    ///
    /// The keys belong to the tile, and the pointer picks which tile a press means (<see cref="TileKeys"/>).
    /// A window-level shortcut would have to invent that rule,
    /// and every candidate for it, the focused tile or the last one touched, adds arrangement state to maintain.
    ///
    /// The handler goes on the window because keys arrive at whatever holds the keyboard, which a tile never does:
    /// taking focus on hover would steal it from whatever was being typed in.
    /// </summary>
    protected override void OnAttachedToVisualTree(VisualTreeAttachmentEventArgs e)
    {
        base.OnAttachedToVisualTree(e);

        _root = TopLevel.GetTopLevel(this);
        _root?.AddHandler(KeyDownEvent, OnKeyPressed, RoutingStrategies.Bubble);
    }

    protected override void OnDetachedFromVisualTree(VisualTreeAttachmentEventArgs e)
    {
        // The handler lives on the window, so leaving the tree takes it off again.
        // A window outlives its cards, and handlers for cards nobody draws would pile up for as long as it is open.
        _root?.RemoveHandler(KeyDownEvent, OnKeyPressed);
        _root = null;

        base.OnDetachedFromVisualTree(e);
    }

    /// <summary>
    /// Tracks the view model's notifications, so a landed level redraws the meter.
    /// Levels arrive on a path of their own fifteen times a second and move two properties.
    /// Redrawing resizes one border and touches nothing else on screen
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

    /// <summary>
    /// Reports the size this card arranged at, in its own pixels.
    ///
    /// The pointer arrives as a fraction of the picture and is drawn in these, so the conversion needs both.
    /// The view model reads the picture's shape off the frames, and this is the only source for the card's size.
    /// </summary>
    protected override Size ArrangeOverride(Size size)
    {
        var arranged = base.ArrangeOverride(size);
        _watched?.SetPictureSize(arranged.Width, arranged.Height);
        return arranged;
    }

    /// <summary>Widens the meter's bar to the level, in pixels of this card.</summary>
    private void Draw()
    {
        if (this.FindControl<Border>("Level") is not { } bar)
        {
            return;
        }

        bar.Width = _watched is { HasLevel: true } tile ? Math.Max(0, Bounds.Width * tile.Level) : 0;
    }

    /// <summary>
    /// Makes the volume slider draggable inside the menu.
    /// A press on a menu item is a click and a click closes the flyout,
    /// so untouched the slider takes one press and the drag lands on a menu already dismissed.
    /// Handling the pointer events keeps them off the item,
    /// <c>StaysOpenOnClick</c> in the markup covering the press that still reaches it.
    /// </summary>
    private void OnVolumePressed(object? sender, PointerPressedEventArgs e) => e.Handled = true;

    /// <summary>
    /// Applies what a key names to the tile under the pointer.
    /// The states the menu's rows name, on the keys those rows print beside them,
    /// so reading the menu once is enough.
    /// Each key names a state rather than a step, so a second press is a round trip instead of a second
    /// arrangement, the volume keys naming the level they want for the same reason.
    /// </summary>
    private void OnKeyPressed(object? sender, KeyEventArgs e)
    {
        // The pointer selects the tile, so a press with it elsewhere belongs to another card.
        // Two cards can draw one stream, a grid slot beneath a window filled with the picture,
        // and only the one under the pointer hit-tests as over.
        if (!IsPointerOver || DataContext is not TileViewModel tile)
        {
            return;
        }

        // In a text box a letter is text.
        // Keys start at whatever holds the keyboard,
        // which separates somebody typing in the settings panel from somebody watching the tile under a pointer.
        if (e.Source is TextBox)
        {
            return;
        }

        if (TileKeys.Command(tile, e) is not { } command)
        {
            return;
        }

        // Refused wherever the row the key names is greyed:
        // a stream without a sound track has no level to move and nothing to silence.
        // The press stays unhandled, a key this tile cannot answer not being its own.
        if (!command.CanExecute(null))
        {
            return;
        }

        command.Execute(null);
        e.Handled = true;
    }

    private void InitializeComponent() => AvaloniaXamlLoader.Load(this);
}
