using System.Globalization;
using Avalonia.Data.Converters;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Controls;

/// <summary>
/// Reads a <see cref="CheckState"/> as the glyph it wears.
/// The table lives on <see cref="CheckItem.GlyphOf"/>; this only carries it into a binding, so the template
/// and any future consumer read the same one table rather than restating the rule.
/// </summary>
public sealed class CheckGlyphConverter : IValueConverter
{
    public static readonly CheckGlyphConverter Instance = new();

    public object Convert(object? value, Type targetType, object? parameter, CultureInfo culture)
    {
        Assert.That(value is CheckState, "a check glyph is read from a check state", value);

        return CheckItem.GlyphOf((CheckState)value!);
    }

    public object ConvertBack(object? value, Type targetType, object? parameter, CultureInfo culture)
        => Assert.Never<object>("a check glyph is never written back", value);
}
