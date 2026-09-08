namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// How the share group is laid out: the kind first, then the one chooser that kind names.
///
/// <b>Placement and nothing else.</b>
/// Which controls the group holds, which entries each offers and which can be picked are the backend's answers,
/// arriving in the resolved form (<c>docs/ipc-api.md</c>, "The rule").
/// What is decided here is the order a reader meets them in, and that two of them are worth a picture.
///
/// <b>The kind leads.</b>
/// A chooser answers which one, which decides nothing until one what is settled,
/// so a screen grid above the kind asks the second question first.
///
/// <b>Each chooser is drawn from the control it is about.</b>
/// The backend hides the two controls the picked kind does not name (<c>backend/internal/form/share.go</c>),
/// so a chooser reading its own field follows the kind with no gate on this side.
///
/// A chooser draws on a reachable control alone.
/// A machine whose desktop picks for itself leaves all three visible and disabled, carrying one reason,
/// and the kind above them states it once.
///
/// The keys live here rather than in the flow for the reason <see cref="QualityLayout"/>'s do:
/// a key spelled at the render function and again at the view model is one string in two places,
/// and the second goes stale.
/// </summary>
public static class ShareLayout
{
    public const string GroupKey = "share";

    /// <summary>One of a screen, a window and a rectangle. Leads the group.</summary>
    public const string KindKey = "publish.share_kind";

    /// <summary>Control the screen grid is a second way to reach.</summary>
    public const string MonitorKey = "publish.monitor";

    /// <summary>Control the window list is a second way to reach.</summary>
    public const string WindowKey = "publish.share_window";

    /// <summary>Rectangle, dragged over the desktop rather than typed.</summary>
    public const string RegionKey = "publish.share_region";
}
