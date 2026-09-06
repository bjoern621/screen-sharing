using Avalonia;
using Avalonia.Controls;
using ScreenShare.App.Features.Insights.Preview.ViewModel;

namespace ScreenShare.App.Features.Insights.Preview.View;

/// <summary>
/// Markup, and the one fact markup cannot state: the size the picture is being drawn at.
///
/// No widget property is read or set here.
/// One view-model input is written, the shape every other input on this screen has, and the render function
/// decides what it means.
///
/// Whether the card draws is the reader's, written through the card's own transport control, so this view watches
/// neither the visual tree nor the window
/// (<c>docs/viewer-architecture.md</c>, "What the insights preview draws").
/// </summary>
public sealed partial class PreviewView : UserControl
{
    public PreviewView() => InitializeComponent();

    /// <summary>
    /// Reports the size the card arranged the picture at, in the card's own pixels.
    ///
    /// The pointer arrives in the picture's pixels and is drawn in these, so the conversion needs both.
    /// The view model reads the picture's size off the frames, and this is the only source for the card's.
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
