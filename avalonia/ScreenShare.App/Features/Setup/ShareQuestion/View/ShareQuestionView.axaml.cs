using Avalonia;
using Avalonia.Controls;
using ScreenShare.App.Features.Setup.ShareQuestion.ViewModel;
using ScreenShare.App.Features.Shell.Model;

namespace ScreenShare.App.Features.Setup.ShareQuestion.View;

/// <summary>
/// Markup, and the one fact the markup cannot state: whether this question is being looked at.
///
/// The answer decides more than a subscription here.
/// The screen chooser's pictures come off a screen capture per monitor the backend opened because this
/// surface asked, so a question nobody is looking at stops reading the screens rather than merely
/// stopping drawing them (<see cref="ShareQuestionViewModel"/>).
///
/// Nothing here sets or reads a widget property: it writes one input of the view model
/// and leaves the render function to decide what that means.
/// </summary>
public sealed partial class ShareQuestionView : UserControl
{
    /// <summary>
    /// Whether this view is on screen in a window that is in front.
    /// Both halves are facts only a control and the platform can answer, so the answer is worked out in the shell
    /// (<see cref="ShowingWatch"/>).
    /// </summary>
    private readonly ShowingWatch _showing;

    /// <summary>View model last told, so the one being left can be told this view stopped drawing it.</summary>
    private ShareQuestionViewModel? _told;

    public ShareQuestionView()
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
    /// Tells the question whether this view is looking at it.
    /// Idempotent, the write it makes being idempotent itself, so a data context that changed without the tree
    /// or the window moving reports the same fact again and converges to the same world.
    /// </summary>
    private void Tell(bool showing)
    {
        var current = DataContext as ShareQuestionViewModel;
        if (!ReferenceEquals(current, _told))
        {
            _told?.SetShowing(false);
            _told = current;
        }

        current?.SetShowing(showing);
    }
}
