using ScreenShare.Api.V1;
using ScreenShare.App.Backend;

namespace ScreenShare.App.Features.Viewer.Tile.Model;

/// <summary>
/// Which protocol a tile's decode is opened on, read off the one field the contract carries it in.
///
/// <b>It exists so the answer is read at one site rather than derived at each row.</b> Every tile the
/// viewer's grid opens has to name a leg when it calls <see cref="IBackend.StartReceiveAsync"/>, and a leg
/// derived per caller would be this module deciding something it may not decide.
///
/// <b>The broadcast screen's preview is its second caller, on one of its two routes.</b> The local route
/// draws a copy the publish child writes to a loopback port, which crossed no protocol at all and has no leg
/// to name; the end-to-end route opens a decode of this machine's own stream off the relay and needs a leg
/// exactly as a viewer tile does (<c>docs/viewer-architecture.md</c>, "What the broadcast preview draws").
/// It reads this field rather than one of its own, because how this machine receives is one setting and a
/// second answer to it on the broadcast screen would be a preview drawn over a protocol the viewer never
/// uses.
///
/// <b>What it reads is a value and not a choice.</b> <c>viewer.tile_watch_transport</c> is a setting,
/// resolved and repaired by the backend against the transport table and the format the relay reports for the
/// path (<c>docs/viewer-architecture.md</c>, "Two legs, two protocols").
/// Every rule about which legs carry which formats on which engine lives behind that resolve; this side reads
/// the answer and never re-derives it.
/// It is a separate field from the player's because the two receivers reach different protocol sets - a tile
/// takes WebRTC that no player opens by address, and a player opens the relay's HLS that no tile reads.
///
/// <b>The answer is read out of the settings the backend holds and not out of the draft.</b> The leg is one
/// of six knobs the watch group carries and it is the only one the shell names in a call: the render chain
/// and both jitter buffers are read by the backend out of its own settings when the decode is built
/// (<c>docs/ipc-api.md</c>).
/// Reading this one off the draft therefore made half a staged panel take effect as it was edited while the
/// other half waited for the commit, and a decode could open on a leg whose latency was the value before it.
/// One source for all six is what removes that.
///
/// An empty answer is settings that have not arrived yet, which leaves a screen saying so rather than
/// guessing a protocol.
/// </summary>
public static class TileLeg
{
    /// <summary>
    /// The field key, which is a settings message and a field in it.
    /// Named once here, so a rename in the contract is one line rather than one line per screen.
    /// </summary>
    public const string Key = "viewer.tile_watch_transport";

    /// <summary>
    /// The leg the settings name, and the empty string before any have been read.
    /// </summary>
    public static string Of(Settings? settings)
    {
        if (settings is null)
        {
            return "";
        }

        return FieldValues.AsText(SettingsDraft.Read(settings, Key));
    }
}
