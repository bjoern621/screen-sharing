using ScreenShare.Api.V1;

namespace ScreenShare.App.Copy;

/// <summary>
/// How the Discord link is named on screen.
///
/// Two surfaces state it, the relay step beside the button that draws it and the settings dialog,
/// and both draw <see cref="State"/>: one fact reads one way wherever a reader meets it.
/// Nothing here reads the draft, so the Follow Discord toggle moves no word of it.
/// </summary>
public static class Links
{
    /// <summary>
    /// The link, carrying the account it was drawn for.
    /// A clause the caller ends, and the bare word for a manager that named no account.
    /// </summary>
    public static string Linked(DiscordState state) =>
        state.AccountName.Length > 0 ? $"Linked as {state.AccountName}" : "Linked";

    /// <summary>
    /// Where this install stands with Discord, in one sentence.
    ///
    /// Four answers, because the reader's next move differs in each: link an account, link again,
    /// nothing, or wait for the manager to answer again (<c>docs/discord-mode.md</c>).
    /// What a voice channel is worth is the form's to refuse, so no answer here asks for one.
    /// </summary>
    /// <param name="state">Null until the backend answers.</param>
    public static string State(DiscordState? state)
    {
        if (state is null)
        {
            return "The link is read once the backend answers.";
        }

        if (!state.Linked)
        {
            return "No Discord account is linked. Link one to follow a voice channel.";
        }

        var linked = Linked(state);
        if (state.LinkRefused)
        {
            return $"{linked}. The Discord manager does not recognize this link. Link Discord again.";
        }

        var line = state.InChannel ? $"{linked}. Following {state.ChannelName} in {state.GuildName}." : $"{linked}.";
        return state.Stale
            ? line + " The Discord manager is not answering, so this may be out of date."
            : line;
    }

    /// <summary>
    /// Whether that sentence reports something broken, which draws it in the failure hue
    /// (<c>docs/design-language.md</c>, "Palette").
    /// A link the manager declines is the one state that is.
    /// </summary>
    public static bool StateIsFailure(DiscordState? state) => state?.LinkRefused ?? false;
}
