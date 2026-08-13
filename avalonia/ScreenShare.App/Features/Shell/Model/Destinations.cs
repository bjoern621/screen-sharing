using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// The destination table: the strip's draw order, and one label per destination.
/// Read rather than restated, which keeps the segmented control, the window title and the body on one
/// vocabulary (docs/development-principles.md, "Static facts belong in a table").
/// </summary>
public static class Destinations
{
    /// <summary>
    /// Left to right, never reordered.
    /// The order is the destinations' own, since nothing broadcasts before it is set up and watching holds
    /// throughout, so it is a fact about the app rather than a preference a screen restates.
    /// </summary>
    public static readonly IReadOnlyList<Destination> All =
    [
        Destination.Setup,
        Destination.Broadcast,
        Destination.Viewer,
    ];

    /// <summary>
    /// A destination added to the enum and not to the order draws a segment short with nothing saying so, so
    /// the table checks itself at first use.
    /// </summary>
    static Destinations()
        => Assert.That(
            All.Count == Enum.GetValues<Destination>().Length,
            "the drawing order names every destination",
            All.Count,
            Enum.GetValues<Destination>().Length);

    /// <summary>
    /// What a segment says, verbatim and in the case the design draws.
    /// Exhaustive, so a destination added without a label fails here rather than drawing blank.
    /// </summary>
    public static string LabelOf(Destination destination) => destination switch
    {
        Destination.Setup => "Setup",
        Destination.Broadcast => "Broadcast",
        Destination.Viewer => "Viewer",
        _ => Assert.Never<string>("unexpected destination", (int)destination),
    };
}
