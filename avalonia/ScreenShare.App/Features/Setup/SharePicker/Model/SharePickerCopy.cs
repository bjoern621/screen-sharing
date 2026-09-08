namespace ScreenShare.App.Features.Setup.SharePicker.Model;

/// <summary>
/// Words the picker owns, past the ones the form group brings with it.
///
/// The heading, the controls and the sentence under a grayed entry come from the share group
/// (<c>Copy/Fields.cs</c>, <c>Copy/Statements.cs</c>), so nothing here repeats them.
/// What is here belongs to the dialog: the two buttons, and the line saying when it appears.
/// </summary>
public static class SharePickerCopy
{
    public const string Title = "Share your screen";

    public const string Help = "Pick what viewers see. The stream starts as soon as you share.";

    public const string Confirm = "Share";

    public const string Cancel = "Cancel";

    public const string CancelTip = "Close without starting a stream";

    /// <summary>Beside the region control, which is drawn on the screen rather than typed.</summary>
    public const string DrawRegion = "Draw a region";

    public const string DrawRegionTip = "Drag a rectangle on any screen. Press Escape to keep the current one.";

    /// <summary>What the region control shows before anything is drawn.</summary>
    public const string NoRegion = "Nothing drawn yet";

    /// <summary>The region as a reader reads one back: the size, then where it sits.</summary>
    public static string Region(int x, int y, int width, int height) =>
        $"{width} × {height} at {x}, {y}";
}
