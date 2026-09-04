using ScreenShare.Api.V1;
using ScreenShare.App.Copy;

namespace ScreenShare.App.Features.Viewer.Model;

/// <summary>
/// What the empty grid says, one sentence per cause, each naming the reader's next step.
///
/// Quiet while a tile is up, while the first presence read is out,
/// and while the relay's own notice on the rail already speaks: an empty grid is not told twice why.
/// </summary>
public static class GridEmpty
{
    public static string For(MembersState? members, int streams, int tiles, bool relayReady)
    {
        if (tiles > 0 || members is null)
        {
            return "";
        }

        if (!members.Joined)
        {
            return Cards.GridOutside;
        }

        if (streams > 0)
        {
            return Cards.GridUnwatched;
        }

        return relayReady ? Cards.GridIdle : "";
    }
}
