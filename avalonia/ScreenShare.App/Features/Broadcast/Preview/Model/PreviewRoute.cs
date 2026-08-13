using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;

namespace ScreenShare.App.Features.Broadcast.Preview.Model;

/// <summary>
/// Which of the two pictures of one stream the broadcast preview draws, and the whole of what the card's
/// toggle chooses.
///
/// Both carry the same encode, so neither answers what the capture looked like before it.
/// What separates them is everything downstream of the encoder: <see cref="Local"/> is taken before the relay
/// and is blind to the uplink, the relay and the viewer's link, and <see cref="EndToEnd"/> crosses all three.
/// A congested uplink is a stutter on one and a perfect picture on the other.
///
/// Neither is a default the other falls back to.
/// A stream with no local preview leg draws nothing on <see cref="Local"/>, a relay not carrying the path
/// draws nothing on <see cref="EndToEnd"/>, and each says which state it is in.
/// </summary>
public enum PreviewRoute
{
    /// <summary>
    /// The copy the publish child writes to a loopback port, decoded here.
    /// One local decode, no bandwidth, and no reader counted at the relay
    /// (<c>docs/viewer-architecture.md</c>, "What the broadcast preview draws").
    /// </summary>
    Local,

    /// <summary>
    /// This machine's own stream pulled back off the relay, over the leg its viewer receives on.
    /// A relay decode like any other: a reader slot, a viewer counted in the figures beside the card, and one
    /// viewer's downstream bandwidth.
    /// </summary>
    EndToEnd,
}

/// <summary>
/// Route table: which routes exist, what each is called, and what each picture costs.
/// One table for the toggle, the render pass and the tests, never a switch restated per site
/// (<c>docs/development-principles.md</c>, "Static facts belong in a table").
/// </summary>
public static class PreviewRoutes
{
    /// <summary>
    /// Segment order, left to right.
    /// Local first, since it is what the card opens on and it spends nothing off the relay.
    /// </summary>
    public static readonly IReadOnlyList<PreviewRoute> All =
    [
        PreviewRoute.Local,
        PreviewRoute.EndToEnd,
    ];

    /// <summary>
    /// Checked once at first use: a route added to the enum and not to the order renders one segment short,
    /// and nothing else says so.
    /// </summary>
    static PreviewRoutes()
        => Assert.That(
            All.Count == Enum.GetValues<PreviewRoute>().Length,
            "the drawing order names every preview route",
            All.Count,
            Enum.GetValues<PreviewRoute>().Length);

    /// <summary>What a segment says. Exhaustive, so a route with no label fails here.</summary>
    public static string LabelOf(PreviewRoute route) => route switch
    {
        PreviewRoute.Local => Cards.PreviewLocalLabel,
        PreviewRoute.EndToEnd => Cards.PreviewEndToEndLabel,
        _ => Assert.Never<string>("unexpected preview route", (int)route),
    };

    /// <summary>
    /// What the picture is, what it costs and what it cannot answer, as the sentence under the card states it.
    /// </summary>
    public static string CostOf(PreviewRoute route) => route switch
    {
        PreviewRoute.Local => Cards.PreviewLocalCost,
        PreviewRoute.EndToEnd => Cards.PreviewEndToEndCost,
        _ => Assert.Never<string>("unexpected preview route", (int)route),
    };
}
