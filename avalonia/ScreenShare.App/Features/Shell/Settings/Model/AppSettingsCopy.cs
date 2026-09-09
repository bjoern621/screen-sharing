using ScreenShare.Api.V1;

namespace ScreenShare.App.Features.Shell.Settings.Model;

/// <summary>
/// What the settings dialog says in its own right, around the controls the fields own.
///
/// Keyed on nothing, like <see cref="Copy.Cards"/>: a control under a heading is keyed by field and takes
/// its words from <see cref="Copy.Fields"/>.
/// </summary>
public static class AppSettingsCopy
{
    public const string Title = "Settings";

    public const string CloseTip = "Closes this dialog. Every setting here is already kept.";

    public const string OpenTip =
        "Opens settings for the app as a whole. Set sharing and watching in Setup and Viewer.";

    public const string ResetLabel = "Reset to defaults";

    public const string ResetHelp =
        "Puts every setting here back to the value a fresh installation starts with.";

    public const string WindowHeading = "Window";

    public const string LogsHeading = "Logs";

    public const string LogsBody = "One log per run, kept on this computer.";

    public const string OpenLogsFolder = "Open logs folder";

    public const string OpenLogsFolderTip = "Opens the folder holding the run logs in the file manager.";

    public const string SendReport = "Send a report";

    public const string SendReportTip =
        "Sends the newest run logs, this computer's details and your settings with every secret removed to the relay.";

    /// <summary>
    /// Carries the name the report was stored under, which is what a reader quotes.
    /// Selectable where it is drawn,
    /// for the reason an error text is (<c>CLAUDE.md</c>, "Every error message is selectable and copyable").
    /// </summary>
    public static string ReportSent(string id) => $"Report sent ({id}). Quote that name when you report the problem.";

    public const string UpdatesHeading = "Updates";

    public const string CheckNow = "Check for updates";

    public const string DiscordHeading = "Discord";

    /// <summary>
    /// Press on a check line about the Discord link, which is fixed here rather than on a wizard step
    /// (<c>Features/Setup/Model/CheckAnchor.cs</c>).
    /// Composed from the two names it leads to, so a rename of either carries.
    /// </summary>
    public static readonly string ToDiscord = $"Go to {Title} · {DiscordHeading}";

    public const string DevelopmentHeading = "Development";

    public const string AboutHeading = "About";

    public const string VersionLabel = "Version";

    /// <summary>
    /// Where this install stands with Discord, in the sentence the relay step states it in as well
    /// (<see cref="Copy.Links.State"/>).
    /// </summary>
    public static string DiscordLine(DiscordState? state) => Copy.Links.State(state);
}
