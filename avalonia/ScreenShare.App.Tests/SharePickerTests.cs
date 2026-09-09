using ScreenShare.App.Backend;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.SharePicker.ViewModel;
using ScreenShare.App.Features.Setup.ShareQuestion.ViewModel;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// What the stream shares, asked at the press that starts one.
///
/// The dialog decides nothing about what is offered: the share group arrives resolved, and what these
/// assert is where the press goes and what the answer leaves in the draft.
/// </summary>
public class SharePickerTests
{
    /// <summary>Inline, so a render pass is over when the call is.</summary>
    private static readonly Action<Action> Inline = action => action();

    /// <summary>
    /// A flow with its picker in hand, since a test both presses the commit and answers the dialog.
    /// The region overlay answers whatever the test hands over, no test drawing on a real desktop.
    /// </summary>
    private static SetupViewModel Flow(
        PublishingBackend backend, out SharePickerViewModel picker, string region = "")
    {
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        var step = new ShareQuestionViewModel(backend, form, session, () => Task.FromResult(region), Inline);
        picker = new SharePickerViewModel(step, form);

        var flow = new SetupViewModel(backend, form, session, picker, Flows.ToDiscordSettings, Inline);
        session.Start();
        session.Stop();
        flow.Apply();
        return flow;
    }

    /// <summary>One control of the question, by the key the backend named it with.</summary>
    private static FieldViewModel Control(SharePickerViewModel picker, string key)
        => picker.Question.Group.Fields.Single(control => control.Key == key);

    /// <summary>Picks one entry of a control, the way a press on its row does.</summary>
    private static void Pick(SharePickerViewModel picker, string key, string value)
        => Control(picker, key).Options.Single(option => option.Value == value).Choose.Execute(null);

    /// <summary>
    /// A desktop that draws its own picker leaves nothing here to answer,
    /// so the press reaches the stream as it did before there was a dialog.
    /// </summary>
    [Fact]
    public void AMachineThatAnswersForItselfIsNeverAsked()
    {
        var backend = new PublishingBackend();
        var flow = Flow(backend, out var picker);

        Assert.False(picker.Asks);

        flow.Review.StartSharingCommand.Execute(null);

        Assert.False(picker.IsOpen);
        Assert.Single(backend.Started);
    }

    /// <summary>The press puts the question up and starts nothing until it is answered.</summary>
    [Fact]
    public void TheStartPressAsksBeforeItPublishes()
    {
        var backend = new PublishingBackend(asksWhatToShare: true);
        var flow = Flow(backend, out var picker);

        Assert.True(picker.Asks);

        flow.Review.StartSharingCommand.Execute(null);

        Assert.True(picker.IsOpen);
        Assert.Empty(backend.Started);
    }

    /// <summary>Dropping the question drops the press: nothing goes on the air unasked.</summary>
    [Fact]
    public void CancellingTheQuestionStartsNothing()
    {
        var backend = new PublishingBackend(asksWhatToShare: true);
        var flow = Flow(backend, out var picker);

        flow.Review.StartSharingCommand.Execute(null);
        picker.CancelCommand.Execute(null);

        Assert.False(picker.IsOpen);
        Assert.Empty(backend.Started);
    }

    /// <summary>Answering it starts the stream on the draft the dialog left behind.</summary>
    [Fact]
    public void AnsweringTheQuestionStartsTheStreamOnWhatWasPicked()
    {
        var backend = new PublishingBackend(asksWhatToShare: true);
        var flow = Flow(backend, out var picker);

        flow.Review.StartSharingCommand.Execute(null);
        Pick(picker, "publish.share_kind", "window");
        picker.Question.Windows.Windows.Single(row => row.Handle == "5150").Select.Execute(null);
        picker.ConfirmCommand.Execute(null);

        Assert.False(picker.IsOpen);
        var started = Assert.Single(backend.Started);
        Assert.Equal("window", started.Publish.ShareKind);
        Assert.Equal("5150", started.Publish.ShareWindow);
    }

    /// <summary>
    /// The press is the one way to the question, so a draft the question would fix still offers it.
    /// The dialog carries the verdict instead: what cannot go on the air cannot be confirmed.
    /// </summary>
    [Fact]
    public void ADraftTheQuestionWouldFixStillOffersThePress()
    {
        var backend = new PublishingBackend(asksWhatToShare: true);
        var flow = Flow(backend, out var picker);

        // A window kind with no window picked, which is what the dialog leaves behind when it is dropped
        // on that kind, and what a stored setting carries after the window it named closed.
        Pick(picker, "publish.share_kind", "window");

        Assert.False(flow.IsPublishable);
        Assert.True(flow.Review.CanStartSharing);

        flow.Review.StartSharingCommand.Execute(null);

        Assert.True(picker.IsOpen);
        Assert.False(picker.CanConfirm);
        Assert.NotEmpty(picker.Blocking);
        Assert.Empty(backend.Started);
    }

    /// <summary>
    /// A rectangle is dragged on the desktop rather than typed, so the overlay's answer is what fills
    /// the control. It writes through the draft, as every other control does.
    /// </summary>
    [Fact]
    public void ADrawnRegionReachesTheDraft()
    {
        var backend = new PublishingBackend(asksWhatToShare: true);
        var flow = Flow(backend, out var picker, region: "100,200,1280x720");

        flow.Review.StartSharingCommand.Execute(null);
        Pick(picker, "publish.share_kind", "region");

        Assert.True(picker.Question.Region.IsVisible);
        picker.Question.Region.DrawCommand.Execute(null);
        picker.ConfirmCommand.Execute(null);

        var started = Assert.Single(backend.Started);
        Assert.Equal("region", started.Publish.ShareKind);
        Assert.Equal("100,200,1280x720", started.Publish.ShareRegion);
    }

    /// <summary>
    /// A gesture the reader dropped writes nothing, so backing out of the overlay leaves the last
    /// rectangle standing rather than clearing it.
    /// </summary>
    [Fact]
    public void ADroppedGestureLeavesTheRegionStanding()
    {
        var backend = new PublishingBackend(asksWhatToShare: true);
        var flow = Flow(backend, out var picker);

        flow.Review.StartSharingCommand.Execute(null);
        Pick(picker, "publish.share_kind", "region");
        Control(picker, "publish.share_region").Text = "0,0,640x480";

        picker.Question.Region.DrawCommand.Execute(null);

        Assert.Equal("640 × 480 at 0, 0", picker.Question.Region.Region);
    }
}
