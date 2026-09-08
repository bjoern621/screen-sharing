namespace ScreenShare.App.Features.Shell.Consent.Model;

/// <summary>
/// What the crash report question says in its own right, around the control the field owns.
///
/// Keyed on nothing, like <see cref="Settings.Model.AppSettingsCopy"/>:
/// the toggle under the paragraph is keyed by field and takes its words from <see cref="Copy.Fields"/>.
/// </summary>
public static class ConsentCopy
{
    public const string Title = "Crash reports";

    /// <summary>
    /// What the field's own help leaves out: a send waits on this answer,
    /// and the answer is reachable afterwards.
    /// </summary>
    public const string Body =
        "Crash logs stay on this computer until this is answered. Change the answer in Settings at any time.";

    public const string Continue = "Continue";
}
