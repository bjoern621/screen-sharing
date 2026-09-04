namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// Which controls a wizard step shows while its fold is closed.
///
/// The floor is what a guest joining a group has to touch: the screen to send, and the group to join.
/// Everything else has a preset or a shipped default behind it, so it folds behind the step's
/// disclosure until asked for (<c>docs/design-language.md</c>, "The wizard's floor").
/// The quality and audio steps lay their own content out and state their own fold
/// (<c>QualityStep</c>, <c>AudioStep</c>); this table serves the generically drawn groups.
/// </summary>
public static class FloorLayout
{
    private static readonly HashSet<string> Floor =
    [
        "publish.monitor",
        "relay.group_key",
        "relay.display_name",
    ];

    /// <summary>Keyed on the template, so an indexed key lands with its family.</summary>
    public static bool OnFloor(string key) => Floor.Contains(Copy.Fields.Template(key));
}
