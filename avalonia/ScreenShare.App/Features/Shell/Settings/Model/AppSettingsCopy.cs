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

    public const string LogsHeading = "Logs";

    public const string LogsBody = "One log per run, kept on this computer.";

    public const string OpenLogsFolder = "Open logs folder";

    public const string OpenLogsFolderTip = "Opens the folder holding the run logs in the file manager.";

    public const string UpdatesHeading = "Updates";

    public const string CheckNow = "Check for updates";

    public const string DiscordHeading = "Discord";

    public const string DevelopmentHeading = "Development";

    public const string AboutHeading = "About";

    public const string VersionLabel = "Version";

    /// <summary>
    /// Where this install stands with Discord, in one sentence.
    ///
    /// Five answers, because the reader's next move differs in each: link an account, link again,
    /// join a channel, nothing, or wait for the manager to answer again (<c>docs/discord-mode.md</c>).
    /// </summary>
    public static string DiscordLine(DiscordState? state)
    {
        if (state is null)
        {
            return "The link is read once the backend answers.";
        }

        if (!state.Linked)
        {
            return "No Discord account is linked. Link one in Setup to follow a voice channel.";
        }

        var linked = Copy.Links.Linked(state);
        // A link the manager declines stands stored,
        // so the account reads on and the sentence names the move that clears the refusal.
        if (state.LinkRefused)
        {
            return $"{linked}. The Discord manager does not recognize this link. "
                + "Press Link Discord in Setup to link again.";
        }

        if (!state.InChannel)
        {
            return Stale(state, $"{linked}. The group follows a voice channel once the account joins one.");
        }

        return Stale(state, $"{linked}. Following {state.GuildName} / {state.ChannelName}.");
    }

    /// <summary>Adds what an unanswered manager means for the line above it.</summary>
    private static string Stale(DiscordState state, string line)
        => state.Stale ? line + " The manager stopped answering, so this is what it last said." : line;
}
