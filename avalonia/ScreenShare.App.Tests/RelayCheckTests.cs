using ScreenShare.Api.V1;
using ScreenShare.App.Controls;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.RelayCheck.Model;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>Relay check: what answers on each leg, drawn beside the settings that address them.</summary>
public sealed class RelayCheckTests
{
    private static async Task<SetupViewModel> FlowAsync()
    {
        var flow = Flows.Setup(new SeededBackend("linux"));
        await flow.Settled;
        return flow;
    }

    /// <summary>
    /// Card belongs to the step holding the relay's address and ports,
    /// that being where a leg nothing reaches is corrected.
    /// </summary>
    [Fact]
    public async Task TheCheckDrawsOnTheRelayStepAlone()
    {
        var flow = await FlowAsync();

        // Keys are taken first: standing on a step renders the strip, converging the collection being walked.
        foreach (var key in flow.Steps.Select(step => step.Key).ToList())
        {
            flow.CurrentStep = key;
            Assert.Equal(key == RelayLayout.GroupKey, flow.RelayCheck.IsVisible);
        }
    }

    /// <summary>
    /// Nothing is dialled on a render:
    /// a check costs seconds against a listener that is not there, so it waits for the press.
    /// </summary>
    [Fact]
    public async Task NothingIsDialledUntilTheButtonIsPressed()
    {
        var flow = await FlowAsync();
        flow.CurrentStep = RelayLayout.GroupKey;

        Assert.Empty(flow.RelayCheck.Legs);
        Assert.False(flow.RelayCheck.HasLegs);
    }

    /// <summary>One row per leg the backend answered with, in the order it answered.</summary>
    [Fact]
    public async Task APressDrawsARowPerLeg()
    {
        var flow = await FlowAsync();
        flow.CurrentStep = RelayLayout.GroupKey;

        flow.RelayCheck.CheckCommand.Execute(null);

        Assert.Equal(SeededBackend.CheckedLegs.Count, flow.RelayCheck.Legs.Count);
        Assert.True(flow.RelayCheck.HasLegs);
    }

    /// <summary>
    /// Three verdicts wear the three marks: a listener that answered, one that did not,
    /// and one nothing dialled, which is a note and never a fault.
    /// </summary>
    [Fact]
    public async Task EachVerdictWearsItsOwnMark()
    {
        var flow = await FlowAsync();
        flow.CurrentStep = RelayLayout.GroupKey;

        flow.RelayCheck.CheckCommand.Execute(null);

        Assert.Equal(
            [CheckState.Passed, CheckState.Blocking, CheckState.Note],
            flow.RelayCheck.Legs.Select(leg => leg.State));
    }

    /// <summary>
    /// A row carries the address it dialled and the listener's own words, what a reader takes into a bug report.
    /// </summary>
    [Fact]
    public async Task ARowCarriesTheAddressAndWhatAnswered()
    {
        var flow = await FlowAsync();
        flow.CurrentStep = RelayLayout.GroupKey;

        flow.RelayCheck.CheckCommand.Execute(null);

        var rtsp = flow.RelayCheck.Legs[0].Text;
        Assert.Contains(Words.RelayLeg("rtsp"), rtsp);
        Assert.Contains("rtsps://relay.test:8322", rtsp);
        Assert.Contains("RTSP/1.0 200 OK", rtsp);
        Assert.Contains("41 ms", rtsp);
    }

    /// <summary>
    /// A leg nothing dialled says why in words rather than in the code the backend sent,
    /// and names no address, there being none to dial.
    /// </summary>
    [Fact]
    public async Task AnUndialledLegSaysWhyInWords()
    {
        var flow = await FlowAsync();
        flow.CurrentStep = RelayLayout.GroupKey;

        flow.RelayCheck.CheckCommand.Execute(null);

        var groups = flow.RelayCheck.Legs[2].Text;
        Assert.Contains(Statements.Of(new Text { Code = TextCode.RelayLegNoRelay }), groups);
        Assert.DoesNotContain(nameof(TextCode.RelayLegNoRelay), groups);
    }

    /// <summary>
    /// Summary counts what did not answer, and counts a leg nothing dialled nowhere:
    /// nothing was asked of it, so it is neither well nor unwell.
    /// </summary>
    [Fact]
    public void TheSummaryCountsOnlyWhatWasDialledAndDidNotAnswer()
    {
        Assert.Equal("1 did not answer", RelayLegRows.SummaryOf(RelayLegRows.Of(SeededBackend.CheckedLegs)));

        var answering = RelayLegRows.Of(SeededBackend.CheckedLegs
            .Where(leg => leg.Verdict != RelayLegVerdict.Unreachable)
            .ToList());
        Assert.Equal("everything answered", RelayLegRows.SummaryOf(answering));
    }

    /// <summary>
    /// A relay that answers on nothing is rows and not a refusal,
    /// so the panel for the backend's own sentence stays down (<c>docs/ipc-api.md</c>, "Errors").
    /// </summary>
    [Fact]
    public async Task ARelayAnsweringNothingIsNoRefusal()
    {
        var flow = await FlowAsync();
        flow.CurrentStep = RelayLayout.GroupKey;

        flow.RelayCheck.CheckCommand.Execute(null);

        Assert.False(flow.RelayCheck.HasRefusal);
        Assert.Equal("", flow.RelayCheck.Refusal);
    }
}
