using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Putting one group of settings back to what a fresh installation holds.
/// A staged group is a proposal a reader can walk away from; an applied group is stored as it is typed and is
/// already what this machine is (<c>form.proto</c>, FieldGroup.applied), so it is the one nothing else
/// restores.
/// The relay is such a group: a reader who changed its port has nowhere else to read the number it serves on.
/// No value is named here, because none is named in the shell: every field carries what it starts as
/// (<c>Field.default_value</c>), and what is asserted is that those are what a reset writes back.
/// </summary>
public sealed class ResetGroupTests
{
    private static readonly Action<Action> Inline = action => action();

    /// <summary>The applied group. <see cref="StreamStep"/> is a staged one, for the negative case.</summary>
    private const string RelayStep = "relay";

    private const string StreamStep = "stream";

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

        // The opening draft, which is a fresh installation's.
        var fresh = flow.Form.Draft!.Clone();

        flow.Form.Write("relay.host", new FieldValue { Text = "elsewhere.example" });
        flow.Form.Write("relay.srt_port", new FieldValue { Number = 9001 });
        flow.Form.Write("relay.api_port", new FieldValue { Number = 9002 });
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

        flow.Form.Write("publish.name", new FieldValue { Text = "bjoern" });
        flow.Form.Write("relay.host", new FieldValue { Text = "elsewhere.example" });
        await flow.Form.Settled;

        flow.Form.Reset(RelayStep);
        await flow.Form.Settled;

        Assert.Equal("bjoern", flow.Form.Draft!.Publish.Name);
    }

    /// <summary>
    /// One change of mind about a group, not a burst of the writes a reader could have made by hand, so every
    /// field lands in the draft before anything is stored.
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

        flow.Setup.CurrentStep = StreamStep;
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
