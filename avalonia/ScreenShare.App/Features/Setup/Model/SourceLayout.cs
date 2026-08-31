namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// What the source step places above its controls, and which control that placement is about.
///
/// <b>Placement and nothing else.</b>
/// Which group holds the screen setting, which entries it offers and which can be picked are the backend's answers,
/// arriving in the resolved form (<c>docs/ipc-api.md</c>, "The rule").
/// What is decided here is that this field is worth a picture,
/// and that the picture goes above the list rather than inside it.
///
/// The keys live here rather than in the flow for the reason <see cref="QualityLayout"/>'s do:
/// a key spelled at the render function and again at the view model is one string in two places,
/// and the second goes stale.
/// </summary>
public static class SourceLayout
{
    public const string GroupKey = "source";

    /// <summary>Control the screen picker is a second way to reach.</summary>
    public const string MonitorKey = "publish.monitor";
}
