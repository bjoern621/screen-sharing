namespace ScreenShare.App.Features.Shell.Settings.Model;

/// <summary>
/// Where the settings dialog stands when it opens.
/// A press that comes to fix one thing arrives at the control fixing it,
/// a dialog opened at its top leaving the reader to find that control themselves.
/// </summary>
public enum SettingsSection
{
    /// <summary>Where a press on the strip's own settings button lands.</summary>
    Top,

    /// <summary>Discord heading, holding the link button.</summary>
    Discord,
}
