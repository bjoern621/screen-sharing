using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// The destination table: the order the strip draws, and the one label per destination.
/// Every consumer reads it instead of restating the rule, which is what keeps the segmented control, the
/// window title and the body speaking one vocabulary (docs/development-principles.md, "Static facts belong in
/// a table").
/// </summary>
public static class Destinations
{
    /// <summary>
    /// Left to right, and never reordered.
    /// The order is the destinations' own - you cannot broadcast before you set up, and you watch while both
    /// are true - so it is a fact about the app rather than a preference a screen may restate.
    /// </summary>
    public static readonly IReadOnlyList<Destination> All =
    [
        Destination.Setup,
        Destination.Broadcast,
        Destination.Viewer,
    ];

    /// <summary>
    /// A destination added to the enum but not to the order would render one segment short and nothing would
    /// say so, so the table checks itself once at first use.
    /// </summary>
    static Destinations()
        => Assert.That(
            All.Count == Enum.GetValues<Destination>().Length,
            "the drawing order names every destination",
            All.Count,
            Enum.GetValues<Destination>().Length);

    /// <summary>
    /// What a segment says, verbatim and in the case the design draws it.
    /// Exhaustive, so a destination added without a label fails here rather than rendering blank.
    /// </summary>
    public static string LabelOf(Destination destination) => destination switch
    {
        Destination.Setup => "Setup",
        Destination.Broadcast => "Broadcast",
        Destination.Viewer => "Viewer",
        _ => Assert.Never<string>("unexpected destination", (int)destination),
    };
}
