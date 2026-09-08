using System.Globalization;

namespace ScreenShare.App.Features.Setup.RegionPicker.Model;

/// <summary>
/// The spelling a rectangle is stored in: "100,200,1280x720",
/// the top-left corner in virtual-desktop pixels, then the size
/// (<c>api/proto/screenshare/v1/settings.proto</c>, <c>share_region</c>).
///
/// The shell's one site for it.
/// The overlay writes what somebody dragged and the step reads it back for display,
/// and two sites spelling one format drift.
/// </summary>
internal static class ShareRegion
{
    private const char Separator = ',';
    private const char SizeSeparator = 'x';

    public static string Format(int x, int y, int width, int height)
        => string.Create(
            CultureInfo.InvariantCulture,
            $"{x}{Separator}{y}{Separator}{width}{SizeSeparator}{height}");

    /// <summary>
    /// Reads one back. False for anything not in this spelling, an unset value included,
    /// which is a rectangle nobody has drawn.
    /// </summary>
    public static bool TryRead(string text, out int x, out int y, out int width, out int height)
    {
        x = y = width = height = 0;

        var parts = text.Split(Separator);
        if (parts.Length != 3)
        {
            return false;
        }

        var size = parts[2].Split(SizeSeparator);
        return size.Length == 2
            && Number(parts[0], out x)
            && Number(parts[1], out y)
            && Number(size[0], out width)
            && Number(size[1], out height);
    }

    private static bool Number(string text, out int value)
        => int.TryParse(text, NumberStyles.AllowLeadingSign, CultureInfo.InvariantCulture, out value);
}
