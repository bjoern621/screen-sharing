namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// What a control on this screen asks for.
/// Every one of them is either live-safe - it changes the stream without tearing it down - or it navigates
/// away; nothing here edits configuration in place, which is the rule the whole screen exists to enforce.
///
/// The screen names the request and does not perform it: there is no publisher behind this UI yet, and when
/// there is, it subscribes rather than being reached into.
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
