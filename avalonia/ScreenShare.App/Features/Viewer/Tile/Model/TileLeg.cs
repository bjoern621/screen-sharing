using ScreenShare.Api.V1;
using ScreenShare.App.Backend;

namespace ScreenShare.App.Features.Viewer.Tile.Model;

/// <summary>
/// Which protocol a tile's decode is opened on, read off the one field the contract carries
/// it in.
///
/// <b>It exists so the answer is read at one site rather than derived at two.</b> Two screens
/// now put a tile on the air - the viewer's grid and the broadcast screen's preview - and both
/// have to name a leg when they call <see cref="IBackend.StartReceiveAsync"/>. A leg picked
/// per screen would be this module deciding something it may not decide, and two screens
/// picking it separately would be that rule written twice: the moment one of them learned
/// about a format the other did not, the same stream would be received over two protocols for
/// no reason a reader could see.
///
/// <b>What it reads is a value and not a choice.</b> <c>viewer.tile_watch_transport</c> is a
/// setting, resolved and repaired by the backend against the transport table and the format
/// the relay reports for the path (<c>docs/viewer-architecture.md</c>, "Two legs, two
/// protocols"). Every rule about which legs carry which formats on which engine lives behind
/// that resolve; this side reads the answer and never re-derives it. It is a separate field
/// from the player's because the two receivers reach different protocol sets - a tile takes
/// WebRTC that no player opens by address, and a player opens the relay's HLS that no tile
/// reads.
///
/// An empty answer is a form that does not carry the field, which leaves a screen saying so
/// rather than guessing a protocol.
/// </summary>
public static class TileLeg
{
    /// <summary>
    /// The field key, which is a settings message and a field in it. Named once here, so a
    /// rename in the contract is one line rather than one line per screen.
    /// </summary>
    public const string Key = "viewer.tile_watch_transport";

    /// <summary>
    /// The leg the form resolved to, and the empty string for a form that does not carry the
    /// field or for no form at all.
    /// </summary>
    public static string Of(Form? form)
    {
        if (form is null)
        {
            return "";
        }

        foreach (var group in form.Groups)
        {
            foreach (var field in group.Fields)
            {
                if (field.Key == Key)
                {
                    return FieldValues.AsText(field.Value);
                }
            }
        }

        return "";
    }
}
