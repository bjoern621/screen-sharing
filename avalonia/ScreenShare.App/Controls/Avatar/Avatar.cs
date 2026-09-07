using Avalonia;
using Avalonia.Controls.Primitives;
using Avalonia.Media;
using Google.Protobuf;

namespace ScreenShare.App.Controls;

/// <summary>
/// Somebody's Discord picture, round, at the height a row of text carries.
///
/// It takes the bytes the backend read and never an address:
/// the shell reaches no network,
/// and a picture is a fact off the contract like every other (<c>docs/ipc-api.md</c>).
/// Bytes that are no picture draw nothing,
/// a CDN answering something else being an Umgebungsfehler the row survives.
///
/// Whether one is drawn at all is the screen's:
/// an avatar with nothing to show still measures its own width,
/// so the layout beside it decides where an empty column goes.
/// </summary>
public sealed class Avatar : TemplatedControl
{
    /// <summary>
    /// The picture as the backend read it, empty until one lands or where nobody named one.
    /// </summary>
    public static readonly StyledProperty<ByteString?> PictureProperty =
        AvaloniaProperty.Register<Avatar, ByteString?>(nameof(Picture));

    /// <summary>What the template draws, null where the bytes are no picture.</summary>
    public static readonly DirectProperty<Avatar, IImage?> DrawnProperty =
        AvaloniaProperty.RegisterDirect<Avatar, IImage?>(nameof(Drawn), o => o.Drawn);

    private IImage? _drawn;

    public ByteString? Picture
    {
        get => GetValue(PictureProperty);
        set => SetValue(PictureProperty, value);
    }

    public IImage? Drawn
    {
        get => _drawn;
        private set => SetAndRaise(DrawnProperty, ref _drawn, value);
    }

    /// <summary>
    /// The one render function: the drawn image follows the bytes, and follows nothing else.
    /// </summary>
    protected override void OnPropertyChanged(AvaloniaPropertyChangedEventArgs change)
    {
        base.OnPropertyChanged(change);

        if (change.Property == PictureProperty)
        {
            Drawn = Pictures.Of(Picture);
        }
    }
}
