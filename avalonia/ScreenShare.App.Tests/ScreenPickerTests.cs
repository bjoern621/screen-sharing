using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ViewModel;
using ScreenShare.App.Features.Viewer.Tile.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The screens the setup wizard offers by their pictures, and what those pictures cost.
///
/// Two things are being held to. The grid is a second way to reach one setting, so picking a
/// screen has to write the same value the list under it writes and nothing else. And a picture
/// is a screen capture the backend runs because this grid asked for it, so the asking has to
/// follow the reader: opened on the step they are standing on, closed when they leave it or the
/// window goes behind, and never asked for twice.
/// </summary>
public sealed class ScreenPickerTests
{
    /// <summary>
    /// Reads every running state once and stops before the reconnect delay, so what the session
    /// found is on it and nothing is left dialling behind the assertions.
    /// </summary>
    private static void Load(Session session)
    {
        session.Start();
        session.Stop();
    }

    /// <summary>
    /// A flow standing on the source step with the window in front, which is the one state the
    /// pictures are drawn in. Both halves are stated rather than assumed: the step is the
    /// reader's position and the showing is the control's answer, and the grid needs both.
    /// </summary>
    private static SetupViewModel OnSourceStep(SeededBackend backend, bool showing = true)
    {
        var session = new Session(backend, action => action());
        Load(session);

        var flow = Flows.Setup(backend, session);
        flow.CurrentStep = SourceLayout.GroupKey;
        flow.Screens.SetShowing(showing);
        return flow;
    }

    /// <summary>
    /// The grid offers what the catalog enumerated, named as this shell names a screen, with the
    /// draft's own screen marked. Every one of those is read through rather than held: the
    /// entries are the form's, the names are the catalog's, and which is picked is the draft's.
    /// </summary>
    [Fact]
    public void TheGridOffersEveryEnumeratedScreenAndMarksTheOneTheDraftNames()
    {
        var flow = OnSourceStep(new SeededBackend("linux"));

        Assert.True(flow.Screens.IsVisible);
        Assert.Equal([0, 1], flow.Screens.Screens.Select(screen => screen.Monitor));
        Assert.All(flow.Screens.Screens, screen => Assert.True(screen.IsEnabled));

        // The name is the size and the refresh rate, which is what a reader matches against
        // their own arrangement. It is written here and never sent by the backend.
        Assert.Contains("2560", flow.Screens.Screens[0].Label);
        Assert.Contains("144", flow.Screens.Screens[0].Label);

        Assert.True(flow.Screens.Screens[0].IsSelected);
        Assert.False(flow.Screens.Screens[1].IsSelected);
    }

    /// <summary>
    /// Picking a screen writes the setting, and the mark follows the draft rather than the
    /// press: what moves the outline is the form answering with the new value, which is the
    /// same path the list under the grid takes.
    /// </summary>
    [Fact]
    public void PickingAScreenWritesTheSettingAndMovesTheMark()
    {
        var backend = new SeededBackend("linux");
        var flow = OnSourceStep(backend);

        flow.Screens.Screens[1].Select.Execute(null);

        Assert.False(flow.Screens.Screens[0].IsSelected);
        Assert.True(flow.Screens.Screens[1].IsSelected);

        // The list under the grid moved with it, which is the whole claim: the two are one
        // value read twice, not two controls that happen to agree.
        var list = flow.CurrentGroup!.Fields.Single(field => field.Key == SourceLayout.MonitorKey);
        Assert.Equal("1", list.Options.Single(option => option.IsSelected).Value);
    }

    /// <summary>
    /// One screen capture per enumerated output while the grid is drawn, and each asked for
    /// once. The repeat is what this is really about: the flow re-renders on every keystroke,
    /// and a converge that asked again on each pass would be a call per screen per keystroke.
    /// </summary>
    [Fact]
    public void EveryScreenIsAskedForOnceWhileTheGridIsDrawn()
    {
        var backend = new SeededBackend("linux");
        var flow = OnSourceStep(backend);

        Assert.Equal([0, 1], backend.Previewed);

        flow.Screens.Render();
        flow.Screens.Render();

        Assert.Equal([0, 1], backend.PreviewStarts);
    }

    /// <summary>
    /// Nothing is read for a reader standing on another step, even with the window in front.
    /// The wizard renders every step's model on every pass, so a grid that opened its captures
    /// whenever it was rendered would read the screens of a reader who is configuring an
    /// encoder.
    /// </summary>
    [Fact]
    public void NoScreenIsReadFromAnotherStep()
    {
        var backend = new SeededBackend("linux");
        var session = new Session(backend, action => action());
        Load(session);

        var flow = Flows.Setup(backend, session);
        flow.CurrentStep = "encoder";
        flow.Screens.SetShowing(true);

        Assert.False(flow.Screens.IsVisible);
        Assert.Empty(flow.Screens.Screens);
        Assert.Empty(backend.Previewed);
        Assert.Empty(backend.PreviewStarts);
    }

    /// <summary>
    /// Leaving the step stops the captures. It is the half that costs something if it is
    /// missed: a picture nobody is looking at goes on grabbing every screen five times a second
    /// for as long as the app is open.
    /// </summary>
    [Fact]
    public void LeavingTheStepStopsReadingTheScreens()
    {
        var backend = new SeededBackend("linux");
        var flow = OnSourceStep(backend);

        Assert.Equal([0, 1], backend.Previewed);

        flow.CurrentStep = "encoder";

        Assert.Empty(backend.Previewed);
        Assert.False(flow.Screens.IsVisible);
    }

    /// <summary>
    /// A window that went behind stops the captures as surely as leaving the step does. The two
    /// facts are separate - the reader's position and whether anybody can see it - and the grid
    /// needs both, so either one turning false is enough.
    /// </summary>
    [Fact]
    public void AWindowThatWentBehindStopsReadingTheScreens()
    {
        var backend = new SeededBackend("linux");
        var flow = OnSourceStep(backend);

        flow.Screens.SetShowing(false);

        Assert.Empty(backend.Previewed);
        Assert.False(flow.Screens.IsVisible);
    }

    /// <summary>
    /// A screen a crashed shell left being read is closed by the next one.
    ///
    /// It is the case the whole state exists for. Previews outlive the window that asked for
    /// them, exactly as decodes do, so nothing else would ever stop one - and a screen captured
    /// five times a second for a window that no longer exists is the leak this feature would
    /// otherwise ship with. The shell learns of it by reading the state when it connects, which
    /// is why that read is on the contract rather than left to the event stream.
    /// </summary>
    [Fact]
    public void AScreenLeftBeingReadByAnEarlierShellIsClosed()
    {
        var backend = new SeededBackend("linux");
        backend.Previewed.Add(1);

        var session = new Session(backend, action => action());
        Load(session);

        Assert.Equal([1], session.PreviewedMonitors.Select(previewed => previewed.Monitor));

        // The flow renders on being built, with nothing being looked at yet, and that pass is
        // where the leftover goes.
        Flows.Setup(backend, session);

        Assert.Empty(backend.Previewed);
    }

    /// <summary>
    /// A machine that cannot read one screen apart from another draws no pictures and says why,
    /// in the backend's own statement. Both halves matter: opening captures that would all be
    /// refused is waste, and an absence with no reason beside it reads as a fault.
    /// </summary>
    [Fact]
    public void AMachineThatCannotShowOneScreenSaysSoAndOpensNothing()
    {
        var backend = new SeededBackend("linux")
        {
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

        var flow = OnSourceStep(backend);

        Assert.False(flow.Screens.IsVisible);
        Assert.Empty(backend.Previewed);
        Assert.True(flow.Screens.HasNotice);
        Assert.Contains("Wayland", flow.Screens.Notice);
    }

    /// <summary>
    /// A screen the backend has not opened yet carries no tile and says it is opening.
    ///
    /// <b>The absent tile is the point, not the sentence.</b> A tile subscribes to frames when
    /// it is drawn, and a subscription naming a screen nothing is reading is refused once and
    /// never retried - so a tile made while the start was still in flight would sit dark for as
    /// long as the reader stayed on the step.
    /// </summary>
    [Fact]
    public void AScreenThatIsNotBeingReadYetCarriesNoTile()
    {
        var flow = OnSourceStep(new SeededBackend("linux"));

        Assert.All(flow.Screens.Screens, screen => Assert.Null(screen.Tile));
        Assert.All(flow.Screens.Screens, screen => Assert.True(screen.HasPlaceholder));
    }

    /// <summary>
    /// A screen the backend reports reading carries a tile that names that screen, and the
    /// picture is drawn from the frame channel's monitor arm rather than from a stream.
    /// </summary>
    [Fact]
    public void AScreenTheBackendIsReadingCarriesATileNamingIt()
    {
        var flow = OnSourceStep(Reading(0, 1));
        var tile = flow.Screens.Screens[1].Tile;

        Assert.NotNull(tile);
        Assert.Equal(TileSourceKind.MonitorPreview, tile.Source.Kind);
        Assert.Equal(1, tile.Source.Monitor);
        Assert.False(flow.Screens.Screens[1].HasPlaceholder);
    }

    /// <summary>
    /// The tiles survive a render pass, which is what keeps a picture from being torn down and
    /// re-subscribed every time the form resolves - and the form resolves on every keystroke.
    /// </summary>
    [Fact]
    public void ARenderPassKeepsTheTilesItAlreadyHas()
    {
        var flow = OnSourceStep(Reading(0, 1));
        var before = flow.Screens.Screens.Select(screen => screen.Tile).ToList();

        Assert.All(before, Assert.NotNull);

        flow.Screens.Render();

        Assert.Equal(before, flow.Screens.Screens.Select(screen => screen.Tile));
    }

    /// <summary>
    /// A fixture that is already reading these screens when the shell connects, which is what
    /// puts a tile on the row: the picture follows what the backend reports rather than what
    /// this shell asked for.
    /// </summary>
    private static SeededBackend Reading(params int[] monitors)
    {
        var backend = new SeededBackend("linux");
        backend.Previewed.AddRange(monitors);
        return backend;
    }
}
