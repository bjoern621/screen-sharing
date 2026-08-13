namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// What a control on this screen asks for.
/// Every member is live-safe, changing the stream without tearing it down, or navigates away.
/// Nothing on this screen edits configuration in place.
///
/// A request is named here and performed elsewhere: whoever acts on one subscribes and is never reached into.
/// </summary>
public enum BroadcastAction
{
    Pause,
    ForceKeyframe,
    Reconnect,
    Stop,
    EditInSetup,
    OpenFullLog,
}
