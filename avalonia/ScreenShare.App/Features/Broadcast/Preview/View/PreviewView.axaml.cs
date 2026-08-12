using Avalonia;
using Avalonia.Controls;
using ScreenShare.App.Features.Broadcast.Preview.ViewModel;
using ScreenShare.App.Features.Shell.Model;

namespace ScreenShare.App.Features.Broadcast.Preview.View;

/// <summary>
/// Markup, and the fact the markup cannot state: whether this card is being looked at.
///
/// <b>It is not a second definition of what the card shows.</b> Nothing here sets a widget
/// property and nothing here reads one. What it does is write one input of the view model -
/// the same shape every other input on this screen has - and let the render function decide
/// what that means.
///
/// What the answer decides is a subscription and never a decode: the preview pipeline goes up
/// with the publish child and down with it, so a card nobody is looking at stops drawing and
/// the stream is untouched (<c>docs/viewer-architecture.md</c>, "What the broadcast preview
/// draws").
///
/// The data context is watched as well as the tree, because the two arrive in no fixed order
/// and a template can hand this view a different card. Whichever view model is being left is
/// told it is no longer showing, which is what keeps frames from being lent to a card nothing
/// is drawing.
/// </summary>
public sealed partial class PreviewView : UserControl
{
    /// <summary>
    /// Whether the card is on screen in a window that is in front. Both halves are facts only a
    /// control and the platform can answer, and both are answered in one place because the
    /// wizard's screen picker asks the same question (<see cref="ShowingWatch"/>).
    /// </summary>
    private readonly ShowingWatch _showing;

    /// <summary>The view model last told, so the one being left can be told it is not showing.</summary>
    private PreviewViewModel? _told;

    public PreviewView()
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
    /// Tells the card whether it is being looked at. Idempotent: the write it makes is
    /// idempotent itself, so a data context that changed without the tree or the window moving
    /// reports the same fact again and converges to the same world.
    /// </summary>
    private void Tell(bool showing)
    {
        var current = DataContext as PreviewViewModel;
        if (!ReferenceEquals(current, _told))
        {
            _told?.SetShowing(false);
            _told = current;
        }

        current?.SetShowing(showing);
    }
}
