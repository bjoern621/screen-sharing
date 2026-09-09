namespace ScreenShare.App.Features.Setup.SharePicker.Model;

/// <summary>
/// Words the dialog owns, past the ones the question inside it brings.
///
/// The heading over the controls, the controls and the sentence under a grayed entry come from the share group
/// and its choosers (<c>Copy/Fields.cs</c>, <c>Setup/ShareQuestion</c>), so nothing here repeats them.
/// What is here belongs to the dialog: the two buttons, and the line saying when it appears.
/// </summary>
public static class SharePickerCopy
{
    public const string Title = "Share your screen";

    public const string Help = "Pick what viewers see. The stream starts as soon as you share.";

    public const string Confirm = "Share";

    public const string Cancel = "Cancel";

    public const string CancelTip = "Close without starting a stream";
}
