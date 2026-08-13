using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// How a measurement reaches the screen, stated once for the whole broadcast surface rather than per render
/// function: an unmeasured figure prints an ellipsis, and two figures sharing a line are joined by a middle
/// dot (docs/design-language.md, "Wording").
///
/// Nothing here rounds towards a prettier number.
/// The caller's format is the design's, so <c>0.00</c> loss keeps both decimals a publisher watches for.
/// </summary>
public static class Figure
{
    /// <summary>What a figure reads as before its first sample. Never a zero: zero is a measurement.</summary>
    public const string NoValue = "…";

    private const string Separator = " · ";

    public static string Of(double? value, string format)
    {
        Assert.That(format.Length > 0, "a figure names the format it prints in", format);

        return value is null ? NoValue : value.Value.ToString(format);
    }

    /// <summary>A count, which has no decimals and so takes no format from the caller.</summary>
    public static string Of(int? value) => value is null ? NoValue : value.Value.ToString("0");

    public static string Join(params string[] parts)
    {
        Assert.That(parts.Length > 0, "a joined line holds at least one figure", parts.Length);

        return string.Join(Separator, parts);
    }
}
