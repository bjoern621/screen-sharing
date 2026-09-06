using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
using ScreenShare.App.Controls;
using ScreenShare.App.Copy;

namespace ScreenShare.App.Features.Setup.RelayCheck.Model;

/// <summary>
/// One line of the relay check.
/// A record, so a pass over an unchanged list compares equal and leaves the bound collection alone.
/// </summary>
public sealed record RelayLegRow
{
    /// <summary>
    /// The leg, where it was dialled and what came back, or why nothing was dialled.
    ///
    /// One string, drawn on the one selectable line a check row has: it holds an address and a listener's own words,
    /// which are what a reader takes into a bug report (<c>CLAUDE.md</c>, "Every error message is selectable
    /// and copyable").
    /// </summary>
    public required string Text { get; init; }

    public required CheckState State { get; init; }

    /// <summary>
    /// The version the listener named for itself, empty where it named none.
    /// Beside the row rather than inside its text: it belongs to the relay and is drawn once,
    /// where the summary speaks for the whole list.
    /// </summary>
    public required string Version { get; init; }
}

/// <summary>
/// The list, built from what the backend answered.
/// Every word comes from <c>Copy/</c>: the leg names from the vocabulary the dropdowns use,
/// and the reason a leg went undialled from the statement code beside it.
/// What a listener said stays as it arrived, being another machine's words rather than this app's
/// (api/proto/screenshare/v1/text.proto).
/// </summary>
public static class RelayLegRows
{
    public static IReadOnlyList<RelayLegRow> Of(IReadOnlyList<RelayLeg> legs)
    {
        Assert.NotNull(legs, "building the list needs the legs the backend answered with");

        return legs
            .Select(leg => new RelayLegRow
            {
                Text = $"{Words.RelayLeg(leg.Leg)} · {DetailOf(leg)}",
                State = StateOf(leg.Verdict),
                Version = leg.HasVersion ? leg.Version : "",
            })
            .ToList();
    }

    /// <summary>
    /// One line about the whole list: "everything answered", "1 did not answer, relay version 0.6.1".
    /// Derived rather than stored, so the summary cannot claim a relay is well while the rows show otherwise.
    /// A leg nothing dialled is counted nowhere: nothing was asked of it, so it is neither well nor unwell.
    /// </summary>
    public static string SummaryOf(IReadOnlyList<RelayLegRow> legs)
    {
        Assert.NotNull(legs, "summarising the list needs the list");

        if (legs.Count == 0)
        {
            return "";
        }

        var silent = legs.Count(leg => leg.State == CheckState.Blocking);
        var counted = silent == 0 ? "everything answered" : $"{silent} did not answer";

        var version = VersionOf(legs);
        return version.Length == 0 ? counted : $"{counted}, relay version {version}";
    }

    /// <summary>
    /// The version the relay answered with, empty where its listeners named none or named several.
    /// A deployment answering two versions is running two, and naming one of them would name
    /// whichever leg answered first.
    /// </summary>
    private static string VersionOf(IReadOnlyList<RelayLegRow> legs)
    {
        var named = legs
            .Select(leg => leg.Version)
            .Where(version => version.Length > 0)
            .Distinct()
            .ToList();

        return named.Count == 1 ? named[0] : "";
    }

    /// <summary>
    /// What the row says: the address and the listener's own words, or the reason nothing was dialled.
    /// The wait rides on the end of a leg that was dialled,
    /// which is what tells a port that refused from one that was never there.
    /// </summary>
    private static string DetailOf(RelayLeg leg)
    {
        if (leg.Verdict == RelayLegVerdict.Unaddressed)
        {
            return Statements.Of(leg.Unaddressed);
        }

        var detail = $"{leg.Address} · {leg.Detail}";
        return leg.HasWaitedMs ? $"{detail} · {leg.WaitedMs} ms" : detail;
    }

    /// <summary>
    /// Exhaustive, so a verdict added to the contract fails here,
    /// rather than taking whatever a default arm would give it.
    ///
    /// A leg nothing dialled is a note and never a fault: the relay binds what it is configured to bind,
    /// and a red mark against it would send a reader looking for a break that is not there.
    /// </summary>
    private static CheckState StateOf(RelayLegVerdict verdict) => verdict switch
    {
        RelayLegVerdict.Reachable => CheckState.Passed,
        RelayLegVerdict.Unreachable => CheckState.Blocking,
        RelayLegVerdict.Unaddressed => CheckState.Note,
        _ => Assert.Never<CheckState>("unexpected relay leg verdict", (int)verdict),
    };
}
