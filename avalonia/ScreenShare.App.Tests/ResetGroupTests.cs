using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Putting one group of settings back to what a fresh installation holds.
/// A staged group is a proposal a reader can walk away from.
/// An applied group (<c>form.proto</c>, FieldGroup.applied) is stored as it is typed,
/// and is already what this machine is.
/// It is the one nothing else restores.
/// The relay is such a group: a reader who changed its port has nowhere else to read the number it serves on.
/// No value is named here, none being named in the shell:
/// every field carries what it starts as (<c>Field.default_value</c>),
/// and asserted is that a reset writes those back.
/// </summary>
public sealed class ResetGroupTests
{
    private static readonly Action<Action> Inline = action => action();

    /// <summary>Applied group. <see cref="SourceStep"/> is a staged one, for the negative case.</summary>
    private const string RelayStep = "relay";

    private const string SourceStep = "source";

    private sealed record Flow(SetupViewModel Setup, FormSession Form, SeededBackend Backend);

    private static async Task<Flow> FlowAsync()
    {
        var backend = new SeededBackend("linux");
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        var setup = new SetupViewModel(backend, form, session, Inline);

        await form.Settled;
        return new Flow(setup, form, backend);
    }

    /// <summary>An address and a port are both settings the reader has no other way back to.</summary>
    [Fact]
    public async Task AResetPutsEveryFieldOfTheGroupBack()
    {
        var flow = await FlowAsync();

        // Opening draft, a fresh installation's.
        var fresh = flow.Form.Draft!.Clone();

        flow.Form.Write("relay.host", new FieldValue { Text = "elsewhere.example" });
        flow.Form.Write("relay.srt_port", new FieldValue { Number = 9001 });
        flow.Form.Write("relay.rtsp_port", new FieldValue { Number = 9002 });
        await flow.Form.Settled;

        flow.Form.Reset(RelayStep);
        await flow.Form.Settled;

        Assert.Equal(fresh.Relay, flow.Form.Draft!.Relay);
    }

    /// <summary>A press on one heading is no opinion about the proposal the other steps are still building.</summary>
    [Fact]
    public async Task AResetLeavesTheOtherGroupsWhereTheyAre()
    {
        var flow = await FlowAsync();

        flow.Form.Write("publish.fps", new FieldValue { Number = 120 });
        flow.Form.Write("relay.host", new FieldValue { Text = "elsewhere.example" });
        await flow.Form.Settled;

        flow.Form.Reset(RelayStep);
        await flow.Form.Settled;

        Assert.Equal(120, flow.Form.Draft!.Publish.Fps);
    }

    /// <summary>
    /// One change of mind about a group, not a burst of the writes a reader could have made by hand,
    /// so every field lands in the draft before anything is stored.
    /// </summary>
    [Fact]
    public async Task AResetOfAnAppliedGroupIsStoredOnce()
    {
        var flow = await FlowAsync();
        var fresh = flow.Form.Draft!.Clone();

        flow.Form.Write("relay.host", new FieldValue { Text = "elsewhere.example" });
        await flow.Form.Settled;

        var stored = flow.Backend.Saved.Count;
        flow.Form.Reset(RelayStep);
        await flow.Form.Settled;

        Assert.Equal(stored + 1, flow.Backend.Saved.Count);
        Assert.Equal(fresh.Relay, flow.Backend.Saved[^1].Relay);

        // Storing and starting are different effects, and only one was asked for.
        Assert.Empty(flow.Backend.Started);
    }

    [Fact]
    public async Task TheOfferFollowsTheFormsAppliedGroups()
    {
        var flow = await FlowAsync();

        flow.Setup.CurrentStep = RelayStep;
        var relay = flow.Setup.CurrentGroup!;
        Assert.True(relay.HasAction);

        flow.Setup.CurrentStep = SourceStep;
        Assert.False(flow.Setup.CurrentGroup!.HasAction);
    }

    /// <summary>An offer rebuilt on every pass would be a button replaced under a pointer resting on it.</summary>
    [Fact]
    public async Task AnUnchangedPassOffersTheSameAction()
    {
        var flow = await FlowAsync();
        flow.Setup.CurrentStep = RelayStep;

        var offered = flow.Setup.CurrentGroup!.Action;
        flow.Setup.Apply();

        Assert.Equal(offered, flow.Setup.CurrentGroup!.Action);
    }

    /// <summary>The button and a typed edit are one path into the draft.</summary>
    [Fact]
    public async Task ThePressPutsTheGroupBack()
    {
        var flow = await FlowAsync();
        var fresh = flow.Form.Draft!.Clone();

        flow.Form.Write("relay.host", new FieldValue { Text = "elsewhere.example" });
        await flow.Form.Settled;

        flow.Setup.CurrentStep = RelayStep;
        flow.Setup.CurrentGroup!.Action!.Command.Execute(null);
        await flow.Form.Settled;

        Assert.Equal(fresh.Relay, flow.Form.Draft!.Relay);
    }
}
