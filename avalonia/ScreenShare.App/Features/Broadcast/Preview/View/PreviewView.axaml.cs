using Avalonia;
using Avalonia.Controls;
using ScreenShare.App.Features.Broadcast.Preview.ViewModel;

namespace ScreenShare.App.Features.Broadcast.Preview.View;

/// <summary>
/// Markup, and one fact the markup cannot state: whether this card is on screen.
///
/// <b>It is not a second definition of what the card shows.</b> Nothing here sets a widget
/// property and nothing here reads one. What it does is write one input of the view model -
/// the same shape every other input on this screen has - and let the render function decide
/// what that means.
///
/// <b>Why the view model cannot read it for itself.</b> The shell renders all three
/// destinations on every pass, because a destination that rendered only while it was showing
/// would come back stale (<c>Features/Shell/ViewModel/ShellViewModel.cs</c>). The preview's
/// picture is not free - every frame is a GPU copy into a lent slot and a message on the frame
/// channel - so a card that subscribed whenever anything went live would charge a reader who
/// never opened this screen. Whether a control is in a visual tree is a fact only the control
/// has, so the control states it.
///
/// What it no longer decides is whether the picture exists at all: the preview pipeline goes
/// up with the publish child and down with it, so what this fact turns on and off is a
/// subscription and never a decode (<c>docs/viewer-architecture.md</c>, "What the broadcast
/// preview draws").
///
/// The data context is watched as well as the tree, because the two arrive in no fixed order
/// and a template can hand this view a different card. Whichever view model is being left is
/// told it is no longer showing, which is what keeps frames from being lent to a card nothing
/// is drawing.
/// </summary>
public sealed partial class PreviewView : UserControl
{
    /// <summary>Whether this control is in a visual tree, which is the fact being reported.</summary>
    private bool _attached;

    /// <summary>The view model last told, so the one being left can be told it is not showing.</summary>
    private PreviewViewModel? _told;

    public PreviewView() => InitializeComponent();

    protected override void OnAttachedToVisualTree(VisualTreeAttachmentEventArgs e)
    {
        base.OnAttachedToVisualTree(e);
        _attached = true;
        Report();
    }

    protected override void OnDetachedFromVisualTree(VisualTreeAttachmentEventArgs e)
    {
        base.OnDetachedFromVisualTree(e);
        _attached = false;
        Report();
    }

    protected override void OnDataContextChanged(EventArgs e)
    {
        base.OnDataContextChanged(e);
        Report();
    }

    /// <summary>
    /// Tells the card whether it is on screen. Idempotent: the write it makes is idempotent
    /// itself, so a data context that changed without the tree moving reports the same fact
    /// again and converges to the same world.
    /// </summary>
    private void Report()
    {
        var current = DataContext as PreviewViewModel;
        if (!ReferenceEquals(current, _told))
        {
            _told?.SetShowing(false);
            _told = current;
        }

        current?.SetShowing(_attached);
    }
}
