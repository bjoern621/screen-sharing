using ScreenShare.Api.V1;
using ScreenShare.App.Backend;

namespace ScreenShare.App.Features.Viewer.Tile.Model;

/// <summary>
/// Protocol a tile's decode is opened on, off the single field the contract carries it in.
///
/// Read at one site rather than derived per row: every tile of the viewer's grid names a leg when it calls
/// <see cref="IBackend.StartReceiveAsync"/>, and a leg derived per caller would be this module deciding something
/// it may not decide.
/// The broadcast preview's end-to-end route reads the same field, opening a decode of this machine's own stream
/// off the relay as a viewer tile does.
/// Its local route crossed no protocol and names no leg (<c>docs/viewer-architecture.md</c>, "What the broadcast
/// preview draws").
///
/// What it reads is a value and not a choice.
/// <c>viewer.tile_watch_transport</c> is a setting, resolved and repaired by the backend against the transport
/// table and the format the relay reports for the path (<c>docs/viewer-architecture.md</c>, "Two legs, two
/// protocols").
/// Every rule about which legs carry which formats on which engine lives behind that resolve.
/// The only leg with a field: a player and a browser page are opened per press on a leg the call names, so
/// neither has a value to keep.
///
/// The answer comes out of the settings the backend holds and never out of the draft.
/// Every other knob of the watch group is read by the backend out of those same settings as the decode is built
/// (<c>docs/ipc-api.md</c>), so a leg off the draft takes effect as it is edited while the rest of the staged
/// panel waits for the commit, and the decode opens on a leg whose latency is the value before it.
///
/// An empty answer is settings that have not arrived, leaving a screen saying so rather than guessing a protocol.
/// </summary>
public static class TileLeg
{
    /// <summary>Named once, so a rename in the contract is one line rather than one per screen.</summary>
    public const string Key = "viewer.tile_watch_transport";

    /// <summary>Leg the settings name, empty before any have been read.</summary>
    public static string Of(Settings? settings)
    {
        if (settings is null)
        {
            return "";
        }

        return FieldValues.AsText(SettingsDraft.Read(settings, Key));
    }
}
