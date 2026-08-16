using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Primitives;
using Avalonia.Data;
using Avalonia.Input;
using Avalonia.Interactivity;

namespace ScreenShare.App.Controls;

/// <summary>
/// Slider that says when the reader has its thumb.
///
/// A sweep writes a value per pointer move, and each of them is a configuration passed through on the way to
/// the one the reader stops on.
/// Whoever holds the draft reads <see cref="Sweeping"/> to take the values and ask about the last of them
/// (<c>Backend/FormSession.cs</c>, <c>docs/settings-editing.md</c>).
///
/// The look is the plain slider's: this adds a reading and no chrome, so it takes the theme keyed on
/// <see cref="Slider"/> rather than carrying one of its own.
/// </summary>
public sealed class SweepSlider : Slider
{
    /// <summary>
    /// True from taking the thumb to letting it go.
    /// Bound onto a view model rather than read off this control, the gesture being the view's to know and the
    /// draft somebody else's to hold.
    /// </summary>
    public static readonly DirectProperty<SweepSlider, bool> SweepingProperty =
        AvaloniaProperty.RegisterDirect<SweepSlider, bool>(
            nameof(Sweeping),
            slider => slider.Sweeping,
            (slider, sweeping) => slider.Sweeping = sweeping,
            defaultBindingMode: BindingMode.OneWayToSource);

    private bool _sweeping;

    public SweepSlider()
    {
        // The thumb's own two edges, taken however the template nests it and whether or not the slider handled
        // them first.
        AddHandler(Thumb.DragStartedEvent, (_, _) => Sweeping = true, handledEventsToo: true);
        AddHandler(Thumb.DragCompletedEvent, (_, _) => Sweeping = false, handledEventsToo: true);
    }

    protected override Type StyleKeyOverride => typeof(Slider);

    public bool Sweeping
    {
        get => _sweeping;
        private set => SetAndRaise(SweepingProperty, ref _sweeping, value);
    }

    /// <summary>
    /// Ends the sweep where the gesture ended without the thumb saying so: a pointer taken by something else, and
    /// a control leaving the tree under a held thumb.
    /// Without it the draft would go on holding its question after the reader stopped asking it.
    /// </summary>
    protected override void OnPointerCaptureLost(PointerCaptureLostEventArgs e)
    {
        base.OnPointerCaptureLost(e);

        Sweeping = false;
    }

    protected override void OnDetachedFromVisualTree(VisualTreeAttachmentEventArgs e)
    {
        base.OnDetachedFromVisualTree(e);

        Sweeping = false;
    }
}
