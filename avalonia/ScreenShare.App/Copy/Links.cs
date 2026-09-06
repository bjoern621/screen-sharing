using ScreenShare.Api.V1;

namespace ScreenShare.App.Copy;

/// <summary>
/// How the Discord link is named on screen.
///
/// Two surfaces state it, the relay step beside the button that draws it and the settings dialog,
/// and each continues the clause with what the state means where it stands.
/// </summary>
public static class Links
{
    /// <summary>
    /// The link, carrying the account it was drawn for.
    /// A clause the caller ends, and the bare word for a manager that named no account.
    /// </summary>
    public static string Linked(DiscordState state) =>
        state.AccountName.Length > 0 ? $"Linked as {state.AccountName}" : "Linked";
}
