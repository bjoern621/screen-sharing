using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Putting one group of settings back to what a fresh installation holds.
///
/// It exists for the group nothing else can restore.
/// A staged group is a proposal, so a reader who dislikes what they typed walks away from it; an applied
/// group is stored as it is typed and is already what this machine is (<c>form.proto</c>,
/// FieldGroup.applied).
/// Where the relay is, is that group - a reader who changed a port has nowhere else to read the number the
/// relay serves on.
///
/// The values are not stated here and are not stated in the shell either.
/// Every field carries what it starts as (<c>Field.default_value</c>), and these tests hold the shell to
/// writing exactly those back rather than to any particular number.
/// </summary>
public sealed class ResetGroupTests
{
    private static readonly Action<Action> Inline = action => action();

    /// <summary>The applied group, and a staged one to check the offer against.</summary>
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

    /// <summary>
    /// The whole group goes back, not the one field that was moved last: an address and a port are both
    /// settings the reader had no other way back to.
    /// </summary>
    [Fact]
    public async Task AResetPutsEveryFieldOfTheGroupBack()
    {
        var flow = await FlowAsync();

        // What a fresh installation holds, which is what this shell opened on.
        var fresh = flow.Form.Draft!.Clone();

        flow.Form.Write("relay.host", new FieldValue { Text = "elsewhere.example" });
        flow.Form.Write("relay.srt_port", new FieldValue { Number = 9001 });
        flow.Form.Write("relay.api_port", new FieldValue { Number = 9002 });
        await flow.Form.Settled;

        flow.Form.Reset(RelayStep);
        await flow.Form.Settled;

        Assert.Equal(fresh.Relay, flow.Form.Draft!.Relay);
    }

    /// <summary>
    /// A reset reaches the group it names and nothing else.
    /// The wizard's other steps are a proposal the reader is still building, and a press on one heading is
    /// not an opinion about the others.
    /// </summary>
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
    /// The group is stored once rather than once per field.
    /// Every field goes into the draft before anything is stored, because the reset is one change of mind
    /// about a group and not a burst of the writes a reader could have made by hand.
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

        // Stored, not started.
        // The two are different effects and only one of them was asked for.
        Assert.Empty(flow.Backend.Started);
    }

    /// <summary>
    /// Which headings offer the press is the form's answer: the groups whose fields are the settings
    /// themselves, and no others.
    /// </summary>
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

    /// <summary>
    /// A pass over an unchanged form produces the same offer, which is what keeps the button from being
    /// rebuilt under a pointer that is on it.
    /// </summary>
    [Fact]
    public async Task AnUnchangedPassOffersTheSameAction()
    {
        var flow = await FlowAsync();
        flow.Setup.CurrentStep = RelayStep;

        var offered = flow.Setup.CurrentGroup!.Action;
        flow.Setup.Apply();

        Assert.Equal(offered, flow.Setup.CurrentGroup!.Action);
    }

    /// <summary>
    /// The press goes through the same write the reader's own edits do, so what the button restores and what
    /// a reader could type are one path into the draft.
    /// </summary>
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
