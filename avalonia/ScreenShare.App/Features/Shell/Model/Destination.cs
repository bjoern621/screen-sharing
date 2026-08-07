namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// The three places a window can be. A closed set on purpose: the nav strip draws all of
/// them at once and dims the one that cannot be reached, so the reader learns the shape of
/// the app from the strip rather than from a destination that appears and disappears.
/// </summary>
public enum Destination
{
    /// <summary>Choosing what to send, before anything is live.</summary>
    Setup,

    /// <summary>Watching what this machine is sending. Reachable only while live.</summary>
    Broadcast,

    /// <summary>Watching what everyone else is sending.</summary>
    Viewer,
}
