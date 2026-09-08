using ScreenShare.Api.V1;
using ScreenShare.App.Controls;

namespace ScreenShare.App.Copy;

/// <summary>
/// How the Discord link is named on screen.
///
/// The settings dialog states it, beside the button that draws it: a link is one per computer
/// and configures no stream, so no wizard step owns it.
/// Nothing here reads the draft, so the Follow Discord toggle moves no word of it.
/// </summary>
public static class Links
{
    /// <summary>
    /// The link, carrying the account it was drawn for.
    /// A clause the caller ends, and the bare word for a manager that named no account.
    ///
    /// The account's picture goes in front of the name it belongs to,
    /// marked for whoever draws the sentence (<see cref="AccountLine"/>).
    /// </summary>
    public static string Linked(DiscordState state) =>
        state.AccountName.Length > 0 ? $"Linked as {AccountLine.Mark}{state.AccountName}" : "Linked";

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

    /// <summary>
    /// What the button says.
    /// A linked install presses it to put another account where the linked one stands,
    /// and a refused link is pressed to draw a fresh secret for the account already named,
    /// so the offer a press carries differs by state (<c>docs/discord-mode.md</c>, "Linking, once per install").
    /// </summary>
    public static string Label(DiscordState? state)
    {
        if (state?.LinkRefused == true)
        {
            return "Link Discord again";
        }

        return state?.Linked == true ? "Link a different account" : "Link Discord";
    }

    /// <summary>
    /// What the button handing the link back says, and what its press does.
    /// One offer whatever the manager answers, the press dropping a stored secret and reaching nobody,
    /// so neither word follows the state <see cref="Label"/> does.
    /// </summary>
    public const string UnlinkLabel = "Unlink Discord";

    public const string UnlinkTip =
        "Removes this computer's Discord link. Following a voice channel needs a link, so link again to use one.";

    /// <summary>What the press does, on the state <see cref="Label"/> follows.</summary>
    public static string Tip(DiscordState? state)
    {
        if (state?.LinkRefused == true)
        {
            return "Opens Discord's consent screen in the browser and links this computer again. "
                + "The fresh link is one the Discord manager accepts.";
        }

        return state?.Linked == true
            ? "Opens Discord's consent screen in the browser and links this computer to another Discord account. "
              + "The account linked now is replaced."
            : "Opens the browser on Discord's consent screen and ties this computer to a Discord account. "
              + "One time per computer. The link is what tells the relay which voice channel this computer follows.";
    }
}
