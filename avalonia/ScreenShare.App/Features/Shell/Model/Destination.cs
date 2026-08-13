namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// Where a window can be.
/// Closed on purpose: the nav strip draws every destination at once and dims the ones out of reach, so the
/// shape of the app is read off the strip rather than off entries that appear and vanish.
/// </summary>
public enum Destination
{
    /// <summary>Configuring what this machine sends, before anything is on the air.</summary>
    Setup,

    /// <summary>What this machine is sending. Reachable only while live.</summary>
    Broadcast,

    /// <summary>What everyone else is sending.</summary>
    Viewer,
}
