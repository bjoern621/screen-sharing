using Avalonia;
using Avalonia.Controls;
using ScreenShare.App.Features.Setup.ScreenPicker.ViewModel;
using ScreenShare.App.Features.Shell.Model;

namespace ScreenShare.App.Features.Setup.ScreenPicker.View;

/// <summary>
/// Markup, and the fact the markup cannot state: whether this grid is being looked at.
///
/// <b>What the answer decides here is more than a subscription.</b> The broadcast preview's pictures come off
/// a pipeline the publish already runs; these come off screen captures the backend opens because this grid
/// asked for them, one per monitor.
/// So a grid nobody is looking at stops reading the screens rather than merely stopping drawing them
/// (<see cref="ScreenPickerViewModel"/>).
///
/// Nothing here sets a widget property and nothing here reads one.
/// It writes one input of the view model and lets the render function decide what it means.
/// </summary>
public sealed partial class ScreenPickerView : UserControl
{
    /// <summary>
    /// Whether the grid is on screen in a window that is in front.
    /// Both halves are facts only a control and the platform can answer, and neither is this screen's own, so
    /// the answer is worked out in the shell (<see cref="ShowingWatch"/>).
    /// </summary>
    private readonly ShowingWatch _showing;

    /// <summary>The view model last told, so the one being left can be told it is not showing.</summary>
    private ScreenPickerViewModel? _told;

    public ScreenPickerView()
    {
        InitializeComponent();
        _showing = new ShowingWatch(this, Tell);
    }

    protected override void OnAttachedToVisualTree(VisualTreeAttachmentEventArgs e)
    {
        base.OnAttachedToVisualTree(e);
        _showing.Attached();
    }

    protected override void OnDetachedFromVisualTree(VisualTreeAttachmentEventArgs e)
    {
        base.OnDetachedFromVisualTree(e);
        _showing.Detached();
    }

    protected override void OnDataContextChanged(EventArgs e)
    {
        base.OnDataContextChanged(e);
        _showing.Repeat();
    }

    /// <summary>
    /// Tells the grid whether it is being looked at.
    /// Idempotent: the write it makes is idempotent itself, so a data context that changed without the tree
    /// or the window moving reports the same fact again and converges to the same world.
    /// </summary>
    private void Tell(bool showing)
    {
        var current = DataContext as ScreenPickerViewModel;
        if (!ReferenceEquals(current, _told))
        {
            _told?.SetShowing(false);
            _told = current;
        }

        current?.SetShowing(showing);
    }
}
