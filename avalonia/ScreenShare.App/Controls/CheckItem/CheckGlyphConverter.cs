using System.Globalization;
using Avalonia.Data.Converters;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Controls;

/// <summary>
/// Binding adapter over <see cref="CheckItem.GlyphOf"/>.
/// Carries the glyph table into a template rather than restating it, so markup and code read one table.
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
