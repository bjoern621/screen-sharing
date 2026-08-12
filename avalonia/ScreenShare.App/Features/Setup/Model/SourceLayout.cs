namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// What the source step places above its controls, and which control that placement is about.
///
/// <b>It is placement and nothing else.</b> Which group holds the screen setting, which entries
/// it offers and which of them can be picked are all the backend's answers, arriving in the
/// resolved form (<c>docs/ipc-api.md</c>, "The rule"). What is decided here is that this one
/// field is worth showing a picture of, and that the picture goes above the list rather than
/// inside it - a judgement about a layout, which is exactly the half the contract leaves to a
/// shell.
///
/// The keys live here rather than in the flow for the reason <see cref="QualityLayout"/>'s do:
/// a key spelled at the render function and again at the view model is one string in two places,
/// and the second one is the one that goes stale.
/// </summary>
public static class SourceLayout
{
    /// <summary>The form group this step draws.</summary>
    public const string GroupKey = "source";

    /// <summary>The control the screen picker is a second way to reach.</summary>
    public const string MonitorKey = "publish.monitor";
}
