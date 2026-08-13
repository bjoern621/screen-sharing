using Avalonia;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Interactivity;
using Avalonia.Markup.Xaml;

using ScreenShare.App.Features.Viewer.Tile.ViewModel;

namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// One tile, wherever it is drawn.
///
/// The code-behind is three things the markup cannot state: the meter's bar is a width in pixels rather than
/// a proportion any panel offers, the volume slider inside the menu has to keep the menu open while it is
/// dragged, and a gesture is not a binding.
/// </summary>
public partial class TileCard : UserControl
{
    public TileCard()
    {
        InitializeComponent();

        // The bar is sized from the level and from this control's own width, so both are watched.
        // It is done here rather than with a converter because a converter has one input and this needs two.
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
    /// Whether the stream this card names is being drawn in another window, so this card draws the plate
    /// saying where the picture went rather than the picture.
    ///
    /// <b>It is the host's fact and not the tile's.</b> A popped-out stream is drawn by two cards at once -
    /// the slot it keeps in the grid and the window it went to - and both read one tile.
    /// A card that took the answer off the tile would therefore put the plate in both of them and the picture
    /// in neither, which is what the pop-out window opened for.
    ///
    /// The plate holds no subscription: the style behind it clears the picture's source, and a source that is
    /// gone ends the channel, gives the pool back and stops the render size being asked for
    /// (<see cref="StreamTile"/>).
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
    /// Listens for the tile's keys on the window this card is drawn in.
    ///
    /// The keys are the tile's rather than the window's, and the pointer is what says which tile a press
    /// means (<see cref="TileKeys"/>).
    /// A shortcut hung off the window would have to invent that rule, and each candidate for it (the focused
    /// tile, the last one touched) is a second arrangement state to keep.
    /// Answering only while the pointer is over the card costs none, and the reader is already pointing at
    /// the tile they mean.
    ///
    /// The handler goes on the window because a key reaches whatever holds the keyboard, and a tile holds it
    /// never: taking focus on hover would take it out of whatever the reader was typing in.
    /// </summary>
    protected override void OnAttachedToVisualTree(VisualTreeAttachmentEventArgs e)
    {
        base.OnAttachedToVisualTree(e);

        _root = TopLevel.GetTopLevel(this);
        _root?.AddHandler(KeyDownEvent, OnKeyPressed, RoutingStrategies.Bubble);
    }

    protected override void OnDetachedFromVisualTree(VisualTreeAttachmentEventArgs e)
    {
        // The handler sits on the window rather than on this card, so leaving the tree has to take it off.
        // The window outlives every card in it: a stream that left the grid drops one and so does each end of
        // fullscreen, and the handlers of cards nobody draws would pile up on the window for as long as it is
        // open.
        _root?.RemoveHandler(KeyDownEvent, OnKeyPressed);
        _root = null;

        base.OnDetachedFromVisualTree(e);
    }

    /// <summary>
    /// Follows the view model's notifications so the meter redraws when a level lands.
    ///
    /// The level arrives on its own path at fifteen a second and moves two properties; this redraws one
    /// border's width from them and touches nothing else on the screen (<c>Backend/Session.cs</c>,
    /// <c>Metered</c>).
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
    /// A press inside a menu item is a click on that item, and a click closes the flyout - so without this
    /// the slider would take one press and the drag would land on a menu that was already gone.
    /// Marking the pointer events handled stops them reaching the item; <c>StaysOpenOnClick</c> in the markup
    /// is the other half, for the press that does get through.
    /// </summary>
    private void OnVolumePressed(object? sender, PointerPressedEventArgs e) => e.Handled = true;

    /// <summary>
    /// Fills a screen with this stream, or gives the screen back.
    ///
    /// The same state the menu's fullscreen row names, on the gesture every video player answers to: a reader
    /// who has just double-clicked a picture is asking for the picture, and finding the state only in a
    /// right-click menu is the reason this is here.
    /// The command decides which window it means, as it does for the menu
    /// (<c>Features/Viewer/Model/TileIntent.cs</c>).
    /// </summary>
    private void OnDoubleTapped(object? sender, TappedEventArgs e)
    {
        if (DataContext is not TileViewModel tile)
        {
            return;
        }

        tile.ToggleFullscreen.Execute(null);
        e.Handled = true;
    }

    /// <summary>
    /// Runs what a key names on the tile the pointer is over.
    ///
    /// The same states the menu's rows name, on the keys those rows print beside them, so a reader who has
    /// read the menu once does not have to open it again.
    /// Each names a state rather than a transition, which is what makes a second press a round trip rather
    /// than a second arrangement; the volume keys name the level they want for the same reason.
    /// </summary>
    private void OnKeyPressed(object? sender, KeyEventArgs e)
    {
        // The pointer decides which tile, so a press with the pointer elsewhere is not this card's.
        // One stream can be drawn by two cards at once (a grid slot under the picture filling the window),
        // and only the one under the pointer is hit-tested as over.
        if (!IsPointerOver || DataContext is not TileViewModel tile)
        {
            return;
        }

        // A letter typed into a text box is text.
        // The event starts at whatever holds the keyboard, so this is what tells a reader typing in the
        // settings panel from one watching the tile their pointer happens to rest on.
        if (e.Source is TextBox)
        {
            return;
        }

        if (TileKeys.Command(tile, e) is not { } command)
        {
            return;
        }

        // A key is refused wherever the row it names is greyed: a stream with no sound track has no volume
        // to move and nothing to silence.
        // The press is left unhandled, because a key this tile cannot answer is not this tile's.
        if (!command.CanExecute(null))
        {
            return;
        }

        command.Execute(null);
        e.Handled = true;
    }

    private void InitializeComponent() => AvaloniaXamlLoader.Load(this);
}
