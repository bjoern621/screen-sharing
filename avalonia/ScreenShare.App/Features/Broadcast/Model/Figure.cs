using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// How a measurement reaches the screen. Two rules the whole broadcast surface obeys,
/// stated once here rather than restated at every render function: a figure that has no
/// value yet prints an ellipsis, and two figures on one line are joined by a middle dot
/// (docs/design-language.md, "Wording").
///
/// Nothing here rounds towards a prettier number. The format string is the design's, so
/// <c>0.00</c> loss keeps both decimals a publisher is watching for.
/// </summary>
public static class Figure
{
    /// <summary>What a figure reads as before its first sample. Never a zero: zero is a measurement.</summary>
    public const string NoValue = "…";

    /// <summary>The separator between two figures that share a line.</summary>
    private const string Separator = " · ";

    public static string Of(double? value, string format)
    {
        Assert.That(format.Length > 0, "a figure names the format it prints in", format);

        return value is null ? NoValue : value.Value.ToString(format);
    }

    /// <summary>A count. Counts have no decimals, so they need no format from the caller.</summary>
    public static string Of(int? value) => value is null ? NoValue : value.Value.ToString("0");

    public static string Join(params string[] parts)
    {
        Assert.That(parts.Length > 0, "a joined line holds at least one figure", parts.Length);

        return string.Join(Separator, parts);
    }
}
