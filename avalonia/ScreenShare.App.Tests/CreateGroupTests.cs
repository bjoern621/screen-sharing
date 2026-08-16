using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A stream published with no key is one anybody may watch, so the box holding the key carries the button that
/// draws one and the two states a reader chooses between stay one control apart.
///
/// Pins the button to a text control: the generic renderer draws an action beside one, so a group is reachable
/// from the window rather than from a command line alone.
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
        var flow = await FlowAsync(new SeededBackend("linux"));

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
        var backend = new SeededBackend("linux");
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
    /// A relay draws the key, so a machine pointed at none has nothing to ask and the press is refused with the
    /// reason where a greyed control's reason is stated rather than answered with a key nothing signed.
    /// </summary>
    [Fact]
    public async Task AMachineWithNoRelaySaysWhyNoKeyCanBeDrawn()
    {
        var flow = await FlowAsync(new SeededBackend("linux") { RelayHost = "" });

        Assert.True(GroupKey(flow).HasAction);
        Assert.False(GroupKey(flow).Action!.Command.CanExecute(null));
        Assert.True(GroupKey(flow).HasActionNotice);
        Assert.Contains("relay address", GroupKey(flow).ActionNotice);

        // The refusal is the button's and not the field's: a key pasted before the relay is named is still a key.
        Assert.True(GroupKey(flow).IsEnabled);
    }
}
