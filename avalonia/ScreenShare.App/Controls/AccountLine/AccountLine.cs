using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Documents;
using Avalonia.Media;
using Google.Protobuf;

namespace ScreenShare.App.Controls;

/// <summary>
/// A sentence naming a Discord account, the account's picture drawn where the sentence marks it.
///
/// The mark is what lets one string carry both:
/// the sentence is composed where the link is named and drawn where it lands,
/// and splitting it into parts would put an account's shape into every path between the two
/// (<c>Copy/Links.cs</c>).
/// A sentence naming nobody, and one whose picture has yet to land, draw as the text alone.
/// </summary>
public sealed class AccountLine : SelectableTextBlock
{
    /// <summary>
    /// Where the picture goes, U+FFFC being the character that stands for an object in text.
    /// Drawn nowhere: a sentence keeping it with no picture to put there has it cut.
    /// </summary>
    public const string Mark = "￼";

    /// <summary>The sentence, marked or not.</summary>
    public static readonly StyledProperty<string> SentenceProperty =
        AvaloniaProperty.Register<AccountLine, string>(nameof(Sentence), "");

    /// <summary>The picture, as the backend read it. Empty until one lands.</summary>
    public static readonly StyledProperty<ByteString?> PictureProperty =
        AvaloniaProperty.Register<AccountLine, ByteString?>(nameof(Picture));

    public string Sentence
    {
        get => GetValue(SentenceProperty);
        set => SetValue(SentenceProperty, value);
    }

    public ByteString? Picture
    {
        get => GetValue(PictureProperty);
        set => SetValue(PictureProperty, value);
    }

    /// <summary>
    /// The one render function: the line follows the sentence and the picture, and nothing else.
    /// </summary>
    protected override void OnPropertyChanged(AvaloniaPropertyChangedEventArgs change)
    {
        base.OnPropertyChanged(change);

        if (change.Property == SentenceProperty || change.Property == PictureProperty)
        {
            Render();
        }
    }

    private void Render()
    {
        var sentence = Sentence ?? "";
        var picture = Picture;

        Inlines ??= [];
        Inlines.Clear();

        var at = sentence.IndexOf(Mark, StringComparison.Ordinal);
        if (at < 0 || picture is null || picture.Length == 0)
        {
            Inlines.Add(new Run(sentence.Replace(Mark, "")));
            return;
        }

        Inlines.Add(new Run(sentence[..at]));
        Inlines.Add(new InlineUIContainer(new Avatar { Picture = picture, Classes = { "inline" } })
        {
            BaselineAlignment = BaselineAlignment.Center,
        });
        Inlines.Add(new Run(sentence[(at + Mark.Length)..]));
    }
}
