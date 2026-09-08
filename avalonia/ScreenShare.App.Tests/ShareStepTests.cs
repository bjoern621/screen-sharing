using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ShareStep.ViewModel;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// What the stream shares, as one question: the kind, then the one chooser it names.
///
/// The kind leading is the whole arrangement.
/// A chooser drawn above it would ask which screen before anything said a screen was being shared,
/// so each chooser follows the control the backend leaves visible under the picked kind.
/// </summary>
public sealed class ShareStepTests
{
    /// <summary>One view of the question, standing in for the control that reports it is on screen.</summary>
    private static readonly object Watcher = new();

    /// <summary>Flow on the share step with the window in front, which is what puts the question up.</summary>
    private static SetupViewModel OnShareStep()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var session = new Session(backend, action => action());
        session.Start();
        session.Stop();

        var flow = Flows.Setup(backend, session);
        flow.CurrentStep = ShareLayout.GroupKey;
        flow.Share.SetShowing(Watcher, true);
        return flow;
    }

    /// <summary>Picks one entry of the kind, the way a press on its card does.</summary>
    private static void PickKind(ShareStepViewModel share, string kind)
        => share.Group.Fields
            .Single(control => control.Key == ShareLayout.KindKey)
            .Options.Single(option => option.Value == kind)
            .Choose.Execute(null);

    /// <summary>
    /// The kind is drawn on its own, above the choosers,
    /// and no target control is left as a plain row beside them.
    /// </summary>
    [Fact]
    public void TheKindLeadsAndNoTargetIsDrawnTwice()
    {
        var share = OnShareStep().Share;

        Assert.NotNull(share.Kind);
        Assert.Equal(ShareLayout.KindKey, share.Kind.Key);

        Assert.DoesNotContain(share.Rest, field => field.Key == ShareLayout.KindKey);
        Assert.DoesNotContain(share.Rest, field => field.Key == ShareLayout.MonitorKey);
    }

    /// <summary>
    /// A session that cannot read one screen apart from another draws no grid,
    /// so the plain list comes back rather than leaving the setting unreachable.
    /// </summary>
    [Fact]
    public void AScreenNoPictureCanBeDrawnForKeepsItsList()
    {
        var backend = new SeededBackend("linux")
        {
            AsksWhatToShare = true,
            NoMonitorPreview = new Text
            {
                Code = TextCode.NoMonitorPreview,
                Args =
                {
                    new TextArg { Name = TextArgName.Os, Id = "linux" },
                    new TextArg { Name = TextArgName.Display, Id = "wayland" },
                },
            },
        };

        var session = new Session(backend, action => action());
        session.Start();
        session.Stop();

        var flow = Flows.Setup(backend, session);
        flow.CurrentStep = ShareLayout.GroupKey;
        flow.Share.SetShowing(Watcher, true);

        Assert.False(flow.Share.Screens.IsVisible);
        Assert.Contains(flow.Share.Rest, field => field.Key == ShareLayout.MonitorKey);
    }

    /// <summary>A screen is picked off a picture of it, and only once a screen is what is being shared.</summary>
    [Fact]
    public void TheScreenGridDrawsUnderTheScreenKindAlone()
    {
        var share = OnShareStep().Share;

        Assert.True(share.Screens.IsVisible);
        Assert.False(share.Windows.IsVisible);
        Assert.False(share.Region.IsVisible);

        PickKind(share, "window");

        Assert.False(share.Screens.IsVisible);
        Assert.True(share.Windows.IsVisible);
        Assert.False(share.Region.IsVisible);

        PickKind(share, "region");

        Assert.False(share.Screens.IsVisible);
        Assert.False(share.Windows.IsVisible);
        Assert.True(share.Region.IsVisible);
    }

    /// <summary>
    /// The screens the grid opened are given back when the reader moves off a screen,
    /// a capture per monitor being what the pictures cost.
    /// </summary>
    [Fact]
    public void MovingOffTheScreenKindStopsReadingTheScreens()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var session = new Session(backend, action => action());
        session.Start();
        session.Stop();

        var flow = Flows.Setup(backend, session);
        flow.CurrentStep = ShareLayout.GroupKey;
        flow.Share.SetShowing(Watcher, true);

        Assert.Equal([0, 1], backend.Previewed);

        PickKind(flow.Share, "window");

        Assert.Empty(backend.Previewed);
    }

    /// <summary>
    /// The form carries handles and the titles behind them cross on a read of their own,
    /// so a reader picks by what the window is called.
    /// </summary>
    [Fact]
    public void TheWindowListNamesAndSizesEveryHandleItOffers()
    {
        var share = OnShareStep().Share;
        PickKind(share, "window");

        Assert.Equal(["4242", "5150"], share.Windows.Windows.Select(row => row.Handle));
        Assert.Equal(["Notes · notepad", "Build log · code"], share.Windows.Windows.Select(row => row.Label));
        Assert.Equal(["800 × 600", "1200 × 900"], share.Windows.Windows.Select(row => row.Size));
        Assert.False(share.Windows.IsEmpty);
    }

    /// <summary>Picking a row writes what the form's own control writes, so the two are one value.</summary>
    [Fact]
    public void PickingAWindowWritesTheSetting()
    {
        var share = OnShareStep().Share;
        PickKind(share, "window");

        share.Windows.Windows.Single(row => row.Handle == "5150").Select.Execute(null);

        Assert.True(share.Windows.Windows.Single(row => row.Handle == "5150").IsSelected);
        Assert.Equal(
            "5150",
            share.Group.Fields
                .Single(control => control.Key == ShareLayout.WindowKey)
                .Options.Single(option => option.IsSelected).Value);
    }

    /// <summary>A rectangle nobody drew reads back as the next step rather than as a value.</summary>
    [Fact]
    public void AnUndrawnRegionSaysWhatToDo()
    {
        var share = OnShareStep().Share;
        PickKind(share, "region");

        Assert.True(share.Region.IsVisible);
        Assert.False(share.Region.HasRegion);
        Assert.Contains("Draw one", share.Region.Region);
    }

    /// <summary>A stored rectangle reads back as its size and where it sits.</summary>
    [Fact]
    public void ADrawnRegionReadsBackAsSizeAndPlace()
    {
        var share = OnShareStep().Share;
        PickKind(share, "region");

        share.Group.Fields.Single(control => control.Key == ShareLayout.RegionKey).Text = "100,200,1280x720";

        Assert.True(share.Region.HasRegion);
        Assert.Equal("1280 × 720 at 100, 200", share.Region.Region);
    }

    /// <summary>
    /// A machine whose desktop draws its own picker leaves nothing here to move,
    /// so the press that starts a stream is never held up by a question offering nothing.
    /// </summary>
    [Fact]
    public void AMachineThatAnswersForItselfAsksNothing()
    {
        var backend = new SeededBackend("linux");
        var session = new Session(backend, action => action());
        session.Start();
        session.Stop();

        var flow = Flows.Setup(backend, session);
        flow.CurrentStep = ShareLayout.GroupKey;

        Assert.False(flow.Share.Asks);
        Assert.False(flow.Share.Screens.IsVisible);
        Assert.False(flow.Share.Windows.IsVisible);
        Assert.False(flow.Share.Region.IsVisible);
    }
}
