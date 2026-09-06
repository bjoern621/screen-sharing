using ScreenShare.Api.V1;

namespace ScreenShare.App.Copy;

/// <summary>
/// Wording for the release published beside the running build.
///
/// Two surfaces read from here.
/// The status band states one short line beside the version, at the width a band has.
/// The dialog behind that line states the same fact at length, and offers the restart.
///
/// Why a copy asks nothing, or installs nothing, comes from the backend as a statement
/// and reads through <see cref="Statements"/>, so no reason is written twice.
/// </summary>
public static class Updates
{
    /// <summary>Tooltip on the version, which is the control that asks.</summary>
    public const string Check = "Check for updates";

    /// <summary>Band line while the release page is being read.</summary>
    public const string Checking = "Checking for updates";

    /// <summary>Band line where the running build is the published one.</summary>
    public const string Current = "Up to date";

    /// <summary>Band line once a release is on disk and a restart is all that is left.</summary>
    public const string Ready = "Update ready";

    /// <summary>Band line where a newer release is published.</summary>
    public const string Available = "Update available";

    /// <summary>Band line while the release is arriving.</summary>
    public static string Downloading(int percent) => $"Downloading update, {percent}%";

    /// <summary>Dialog title, which names the release it is about.</summary>
    public static string Title(string version) => $"{Named(version)} is ready";

    /// <summary>Dialog title where the app does not install the release itself.</summary>
    public static string TitleAvailable(string version) => $"{Named(version)} is available";

    /// <summary>
    /// Dialog paragraph over the restart, the one sentence the whole flow exists for.
    /// It names what already happened and what the reader's press does.
    /// </summary>
    public const string Restart =
        "The update is downloaded. It installs when MirrorMe restarts.";

    /// <summary>Dialog paragraph where the reader installs the release themselves.</summary>
    public static string ByHand(string running) =>
        $"You are running {Named(running)}.";

    /// <summary>Button closing the dialog and leaving the release staged.</summary>
    public const string Later = "Later";

    /// <summary>Button that closes the app so the staged release installs.</summary>
    public const string RestartNow = "Restart now";

    /// <summary>Button opening the release page in the browser.</summary>
    public const string OpenPage = "Open release page";

    /// <summary>Button closing a dialog that offers nothing else.</summary>
    public const string Close = "Close";

    /// <summary>
    /// One short line for the band, and the empty string where the band says nothing.
    /// A build nobody stamped and an install that asks nothing both fall here.
    /// </summary>
    public static string Line(UpdateState? state) => state?.Stage switch
    {
        UpdateStage.Checking => Checking,
        UpdateStage.Current => Current,
        UpdateStage.Available => Available,
        UpdateStage.Fetching => Downloading(state.Percent),
        UpdateStage.Ready => Ready,
        UpdateStage.Failed => Statements.Of(state.Failure),
        _ => "",
    };

    /// <summary>
    /// A version as a reader reads one, with the "v" the tags wear.
    /// An empty version reads as the app's own name, so a sentence never opens on a blank.
    /// </summary>
    private static string Named(string version)
    {
        if (version.Length == 0)
        {
            return "A new version";
        }
        return version.StartsWith('v') ? version : "v" + version;
    }
}
