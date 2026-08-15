using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A stream published with no key is one anybody may watch, so the box holding the key carries the button that
/// draws one and the two states a reader chooses between stay one control apart.
///
/// Pins the defect that shipped: the action was composed, bound and never drawn, the generic renderer having
/// hosted it inside the number branch alone, so the only way to a group was a command line.
/// </summary>
public sealed class CreateGroupTests
{
    private static async Task<SetupViewModel> FlowAsync(SeededBackend backend)
    {
        var flow = Flows.Setup(backend);
        await flow.Settled;
        return flow;
    }

    /// <summary>The key control, reached by moving the flow to the step the form puts the relay on.</summary>
    private static FieldViewModel GroupKey(SetupViewModel flow)
    {
        flow.CurrentStep = "relay";
        return flow.CurrentGroup!.Fields.Single(field => field.Key == RelayLayout.GroupKeyKey);
    }

    [Fact]
    public async Task TheGroupKeyCarriesTheButtonThatDrawsOne()
    {
        var flow = await FlowAsync(new SeededBackend("linux") { RelayTls = true });

        Assert.True(GroupKey(flow).HasAction);
        Assert.Equal("Create group", GroupKey(flow).Action!.Label);
        Assert.True(GroupKey(flow).Action!.Command.CanExecute(null));
        Assert.False(GroupKey(flow).HasActionNotice);
    }

    /// <summary>
    /// A drawn key takes the path a pasted one takes: one value in the field, and the relay group being
    /// applied, one save.
    /// </summary>
    [Fact]
    public async Task ADrawnKeyLandsInTheFieldAndIsStored()
    {
        var backend = new SeededBackend("linux") { RelayTls = true };
        var flow = await FlowAsync(backend);

        GroupKey(flow).Action!.Command.Execute(null);
        await flow.Settled;

        Assert.Equal(SeededBackend.DrawnGroupKey, GroupKey(flow).Text);
        Assert.Contains(SeededBackend.DrawnGroupKey, backend.Saved.Select(saved => saved.Relay.GroupKey));

        // The id and not the key: what stands beside the button is news about the attempt, and a second copy
        // of the secret is what the field itself already shows.
        Assert.Contains(SeededBackend.DrawnGroupId, GroupKey(flow).ActionNotice);
        Assert.DoesNotContain(SeededBackend.DrawnGroupKey, GroupKey(flow).ActionNotice);
    }

    /// <summary>
    /// A relay reached without a TLS proxy runs no group service, so the press is refused with the reason
    /// where a greyed control's reason is stated rather than answered with a key nothing signed.
    /// </summary>
    [Fact]
    public async Task ARelayWithNoGroupServiceSaysWhyNoKeyCanBeDrawn()
    {
        var flow = await FlowAsync(new SeededBackend("linux"));

        Assert.True(GroupKey(flow).HasAction);
        Assert.False(GroupKey(flow).Action!.Command.CanExecute(null));
        Assert.True(GroupKey(flow).HasActionNotice);
        Assert.Contains("TLS proxy", GroupKey(flow).ActionNotice);

        // The refusal is the button's and not the field's: a key held on a LAN relay is still a key to paste.
        Assert.True(GroupKey(flow).IsEnabled);
    }
}
