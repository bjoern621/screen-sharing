using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;

namespace ScreenShare.App.Features.Broadcast.Preview.Model;

/// <summary>
/// Which of the two pictures of one stream the broadcast preview draws, and the whole of what the card's
/// toggle chooses.
///
/// <b>They differ by where the picture is taken, which is the only thing a preview can be wrong about.</b>
/// Both carry the same encode, so neither answers what the capture looked like before it.
/// What separates them is everything downstream of the encoder: the local route is taken before the relay and
/// is blind to the uplink, the relay and the viewer's link, and the end-to-end route crosses all three.
/// A congested uplink is a stutter on one and a perfect picture on the other.
///
/// <b>Neither is a default the other falls back to.</b> A stream with no local preview leg draws nothing on
/// <see cref="Local"/>, and a relay that is not carrying the path draws nothing on <see cref="EndToEnd"/>.
/// Each says which state it is in, because substituting the other picture would answer a question the reader
/// did not ask.
/// </summary>
public enum PreviewRoute
{
    /// <summary>
    /// The copy the publish child writes to a loopback port, decoded here.
    /// It costs one local decode, spends no bandwidth, and the relay counts no reader for it
    /// (<c>docs/viewer-architecture.md</c>, "What the broadcast preview draws").
    /// </summary>
    Local,

    /// <summary>
    /// This machine's own stream pulled back off the relay, over the leg its viewer receives on.
    /// It is a relay decode like any other: it occupies a reader slot, the relay counts it among the viewers
    /// reported beside the card, and it costs the downstream bandwidth of one viewer.
    /// </summary>
    EndToEnd,
}

/// <summary>
/// The route table: which routes exist, what each is called, and what each picture is.
///
/// One table, read by the toggle, the render pass and the tests, rather than a switch restated at each of
/// them (<c>docs/development-principles.md</c>, "Static facts belong in a table").
/// </summary>
public static class PreviewRoutes
{
    /// <summary>
    /// Left to right, and never reordered.
    /// Local first, because it is what the card opens on: it costs nothing off the relay, so the route that
    /// spends a reader slot is the one a reader asks for by name.
    /// </summary>
    public static readonly IReadOnlyList<PreviewRoute> All =
    [
        PreviewRoute.Local,
        PreviewRoute.EndToEnd,
    ];

    /// <summary>
    /// A route added to the enum but not to the order would render one segment short and nothing would say
    /// so, so the table checks itself once at first use.
    /// </summary>
    static PreviewRoutes()
        => Assert.That(
            All.Count == Enum.GetValues<PreviewRoute>().Length,
            "the drawing order names every preview route",
            All.Count,
            Enum.GetValues<PreviewRoute>().Length);

    /// <summary>What a segment says. Exhaustive, so a route added without a label fails here.</summary>
    public static string LabelOf(PreviewRoute route) => route switch
    {
        PreviewRoute.Local => Cards.PreviewLocalLabel,
        PreviewRoute.EndToEnd => Cards.PreviewEndToEndLabel,
        _ => Assert.Never<string>("unexpected preview route", (int)route),
    };

    /// <summary>
    /// What this picture is, what it costs, and what it cannot answer, as the sentence under the card states
    /// it.
    /// </summary>
    public static string CostOf(PreviewRoute route) => route switch
    {
        PreviewRoute.Local => Cards.PreviewLocalCost,
        PreviewRoute.EndToEnd => Cards.PreviewEndToEndCost,
        _ => Assert.Never<string>("unexpected preview route", (int)route),
    };
}
