namespace ScreenShare.App.Features.Tray.Model;

/// <summary>
/// The tray's fixed words.
/// Keyed on nothing the backend sends, so it sits with the feature rather than in <c>Copy/</c>,
/// the layering <c>Features/Setup/Model/CommitCopy.cs</c> states.
/// The commit row's words are elsewhere on purpose: the start label is <c>CommitCopy</c>'s and the stop
/// label the broadcast screen's, one place per word.
/// </summary>
public static class TrayCopy
{
    /// <summary>What hovering the icon says. The name alone: the icon itself carries the live state.</summary>
    public const string Tip = "mirrorme";

    /// <summary>Heading of the submenu, the word the rail's card uses for the same list.</summary>
    public const string Presets = "Presets";

    public const string Open = "Open mirrorme";

    /// <summary>The one full-shutdown path: window, tray and the backend this shell started.</summary>
    public const string Quit = "Quit mirrorme";
}
