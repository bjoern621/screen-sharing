namespace ScreenShare.App.Features.Insights.Model;

/// <summary>
/// What a control on this screen asks for.
/// Every member is live-safe, changing the stream without tearing it down, or navigates away.
/// Nothing on this screen edits configuration in place.
///
/// Named here and performed elsewhere: whoever acts on one subscribes and is never reached into.
/// </summary>
public enum InsightsAction
{
    Stop,
    EditInSetup,
    OpenFullLog,
}
