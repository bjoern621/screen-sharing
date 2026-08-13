using Avalonia;
using Avalonia.Controls;
using ScreenShare.App.Features.Broadcast.Preview.ViewModel;

namespace ScreenShare.App.Features.Broadcast.Preview.View;

/// <summary>
/// Markup, and the one fact the markup cannot state: how large the picture is being drawn.
///
/// <b>It is not a second definition of what the card shows.</b> Nothing here sets a widget property and
/// nothing here reads one.
/// What it does is write one input of the view model - the same shape every other input on this screen has -
/// and let the render function decide what that means.
///
/// Whether the card draws is the reader's, written through the card's own transport control, so this view
/// watches neither the visual tree nor the window
/// (<c>docs/viewer-architecture.md</c>, "What the broadcast preview draws").
/// </summary>
public sealed partial class PreviewView : UserControl
{
    public PreviewView() => InitializeComponent();

    /// <summary>
    /// Tells the card how large it is drawing the picture.
    ///
    /// The pointer arrives in the picture's own pixels and is drawn in this card's, so somebody has to know
    /// both.
    /// The view model knows the picture's size off the frames; this is where the card's comes from, and it is
    /// written the same way every other input here is - one value in, and the render function decides what it
    /// means.
    /// </summary>
    protected override Size ArrangeOverride(Size size)
    {
        var arranged = base.ArrangeOverride(size);
        if (DataContext is PreviewViewModel card)
        {
            card.SetPictureSize(arranged.Width, arranged.Height);
        }
        return arranged;
    }
}
