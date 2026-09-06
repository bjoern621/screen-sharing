using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;

namespace ScreenShare.App.Features.Insights.Preview.Model;

/// <summary>
/// Which picture of one stream the insights preview draws, or that it draws none, and the whole of what
/// the card's toggle chooses.
///
/// The two routes carry the same encode, so neither answers what the capture looked like before it.
/// What separates them is everything downstream of the encoder: <see cref="Local"/> is taken before the relay and
/// is blind to the uplink, the relay and the viewer's link, and <see cref="EndToEnd"/> crosses all three.
/// A congested uplink is a stutter on one and a perfect picture on the other.
///
/// Neither is a default the other falls back to.
/// A stream with no local preview leg draws nothing on <see cref="Local"/>, a relay not carrying the path draws
/// nothing on <see cref="EndToEnd"/>, and each says which state it is in.
///
/// <see cref="Off"/> is a state of this card and reaches no stream, so it stands beside the two rather than under
/// a control of its own: where the picture is taken has an answer of "nowhere".
/// </summary>
public enum PreviewRoute
{
    /// <summary>
    /// No picture and no decode: no tile subscribes, and the relay decode the end-to-end route holds is closed
    /// and its reader slot given back.
    /// The publish is untouched, the local preview leg belonging to the child that encodes the stream.
    /// </summary>
    Off,

    /// <summary>
    /// Copy the publish child writes to a loopback port, decoded here.
    /// One local decode, no bandwidth, and no reader counted at the relay (<c>docs/viewer-architecture.md</c>,
    /// "What the insights preview draws").
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
    /// By what each costs: nothing, one local decode, then a reader slot on the relay.
    /// </summary>
    public static readonly IReadOnlyList<PreviewRoute> All =
    [
        PreviewRoute.Off,
        PreviewRoute.Local,
        PreviewRoute.EndToEnd,
    ];

    /// <summary>
    /// Checked once at first use: a route added to the enum and not to the order renders one segment short, and
    /// nothing else says so.
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
        PreviewRoute.Off => Cards.PreviewOffLabel,
        PreviewRoute.Local => Cards.PreviewLocalLabel,
        PreviewRoute.EndToEnd => Cards.PreviewEndToEndLabel,
        _ => Assert.Never<string>("unexpected preview route", (int)route),
    };

    /// <summary>What the picture is, what it costs and what it cannot answer, as the card's sentence states
    /// it.</summary>
    public static string CostOf(PreviewRoute route) => route switch
    {
        PreviewRoute.Off => Cards.PreviewOffCost,
        PreviewRoute.Local => Cards.PreviewLocalCost,
        PreviewRoute.EndToEnd => Cards.PreviewEndToEndCost,
        _ => Assert.Never<string>("unexpected preview route", (int)route),
    };

    /// <summary>
    /// Settings field the chosen route is stored in, so a card opens on the route it was left on.
    /// Named once, so a rename in the contract is one line rather than one per screen.
    /// </summary>
    public const string Key = "viewer.preview_route";

    /// <summary>What the settings spell for a route. Exhaustive, so a route with no value fails here.</summary>
    public static string ValueOf(PreviewRoute route) => route switch
    {
        PreviewRoute.Off => "off",
        PreviewRoute.Local => "local",
        PreviewRoute.EndToEnd => "end-to-end",
        _ => Assert.Never<string>("unexpected preview route", (int)route),
    };

    /// <summary>
    /// Route the settings name, and <see cref="Opening"/> for settings that have not arrived.
    /// A stored value no route carries is repaired as the file is read
    /// (<c>backend/internal/settings/migrate.go</c>),
    /// so a card meeting one anyway opens rather than crashing over a file it does not own.
    /// </summary>
    public static PreviewRoute Of(Settings? settings)
    {
        if (settings is null)
        {
            return Opening;
        }

        var value = FieldValues.AsText(SettingsDraft.Read(settings, Key));
        foreach (var route in All)
        {
            if (ValueOf(route) == value)
            {
                return route;
            }
        }

        return Opening;
    }

    /// <summary>
    /// Route a card draws while its settings are being read.
    /// The picture that costs nothing beyond one decode here,
    /// which is what a fresh installation is stored on (<c>backend/internal/settings</c>, Defaults).
    /// Drawn only: the stored route replaces it when the read lands,
    /// and the reader's selection is what stores a route.
    /// </summary>
    private const PreviewRoute Opening = PreviewRoute.Local;
}
