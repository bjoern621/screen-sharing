using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The group key is the value a reader has to get out of the window and to somebody else,
/// so the box holding it carries the button that puts it on the clipboard.
/// Pins the offer to that one control and to a box with something in it:
/// the generic renderer draws every text field, and a button over an empty box copies nothing.
/// </summary>
public sealed class CopyFieldTests
{
    private static async Task<SetupViewModel> FlowAsync(SeededBackend backend)
    {
        var flow = Flows.Setup(backend);
        await flow.Settled;
        return flow;
    }

    /// <summary>One control of the relay step, reached by moving the flow to it.</summary>
    private static FieldViewModel Field(SetupViewModel flow, string key)
    {
        flow.CurrentStep = "relay";
        return flow.CurrentGroup!.Fields.Single(field => field.Key == key);
    }

    [Fact]
    public async Task AnEmptyKeyBoxOffersNoCopy()
    {
        var flow = await FlowAsync(new SeededBackend("linux"));

        Assert.Equal("", Field(flow, RelayLayout.GroupKeyKey).Text);
        Assert.False(Field(flow, RelayLayout.GroupKeyKey).CanCopy);
    }

    [Fact]
    public async Task AKeyInTheBoxIsOfferedToTheClipboard()
    {
        var flow = await FlowAsync(new SeededBackend("linux"));

        Field(flow, RelayLayout.GroupKeyKey).Action!.Command.Execute(null);
        await flow.Settled;

        Assert.True(Field(flow, RelayLayout.GroupKeyKey).CanCopy);
    }

    /// <summary>
    /// The relay's address is text with something in it, and it carries no button:
    /// the offer is placement rather than a treatment every box takes.
    /// </summary>
    [Fact]
    public async Task ABoxHoldingSomethingElseOffersNoCopy()
    {
        var flow = await FlowAsync(new SeededBackend("linux"));

        Assert.NotEqual("", Field(flow, "relay.host").Text);
        Assert.False(Field(flow, "relay.host").CanCopy);
    }
}
