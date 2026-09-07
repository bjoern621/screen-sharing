using ScreenShare.Api.V1;
using ScreenShare.App.Copy;

namespace ScreenShare.App.Features.Viewer.Members.Model;

/// <summary>
/// What the member list says in place of rows, one sentence per cause, each naming the reader's next step.
///
/// Quiet while there are rows to show.
/// </summary>
public static class MembersEmpty
{
    public static string For(MembersState? members, DiscordState? discord, bool discordMode, int rows)
    {
        if (rows > 0)
        {
            return "";
        }

        if (members is null)
        {
            return Cards.MembersUnread;
        }

        if (!members.Joined)
        {
            // In Discord mode the next step is Discord's rather than a key:
            // link once, then stand in a voice channel.
            if (discordMode)
            {
                return discord is { Linked: true } ? Cards.MembersNoChannel : Cards.MembersUnlinked;
            }
            return Cards.MembersOutside;
        }

        return Cards.MembersNone;
    }
}
