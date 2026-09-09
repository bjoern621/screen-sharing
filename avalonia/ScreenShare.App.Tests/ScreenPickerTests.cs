using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.SharePicker.ViewModel;
using ScreenShare.App.Features.Setup.ViewModel;
using ScreenShare.App.Features.Viewer.Tile.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Grid is a second path to <c>publish.monitor</c>, so picking writes what the list writes and nothing else.
/// Each picture costs a backend capture,
/// so it is asked for once per screen, while the dialog is up with the window in front.
/// </summary>
public sealed class ScreenPickerTests
{
    /// <summary>
    /// Reads every state once and stops before the reconnect delay, so nothing is left dialling behind the assertions.
    /// </summary>
    private static void Load(Session session)
    {
        session.Start();
        session.Stop();
    }

    /// <summary>
    /// Dialog up with the window in front, the only state the grid draws pictures in.
    /// The press that opens it waits on an answer no test gives, so the returned dialog stands open.
    /// </summary>
    private static SharePickerViewModel Asked(SeededBackend backend, bool showing = true)
    {
        var session = new Session(backend, action => action());
        Load(session);

        var picker = Flows.Picker(backend, session);
        picker.AskAsync();
        picker.Question.SetShowing(showing);
        return picker;
    }

    [Fact]
    public void TheGridOffersEveryEnumeratedScreenAndMarksTheOneTheDraftNames()
    {
        var picker = Asked(new SeededBackend("linux") { AsksWhatToShare = true });

        Assert.True(picker.Question.Screens.IsVisible);
        Assert.Equal([0, 1], picker.Question.Screens.Screens.Select(screen => screen.Monitor));
        Assert.All(picker.Question.Screens.Screens, screen => Assert.True(screen.IsEnabled));

        // Label is composed here from the catalog row, the backend sending no name for a screen.
        Assert.Contains("2560", picker.Question.Screens.Screens[0].Label);
        Assert.Contains("144", picker.Question.Screens.Screens[0].Label);

        Assert.True(picker.Question.Screens.Screens[0].IsSelected);
        Assert.False(picker.Question.Screens.Screens[1].IsSelected);
    }

    /// <summary>Mark follows the draft rather than the press, the form answering with the value being what moves it.</summary>
    [Fact]
    public void PickingAScreenWritesTheSettingAndMovesTheMark()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var picker = Asked(backend);

        picker.Question.Screens.Screens[1].Select.Execute(null);

        Assert.False(picker.Question.Screens.Screens[0].IsSelected);
        Assert.True(picker.Question.Screens.Screens[1].IsSelected);

        // Grid and list are one value read twice, not two controls kept in step.
        var list = picker.Question.Group.Fields.Single(field => field.Key == ShareLayout.MonitorKey);
        Assert.Equal("1", list.Options.Single(option => option.IsSelected).Value);
    }

    /// <summary>
    /// Flow re-renders on every keystroke,
    /// so a converge asking again on each pass costs a call per screen per keystroke.
    /// </summary>
    [Fact]
    public void EveryScreenIsAskedForOnceWhileTheGridIsDrawn()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var picker = Asked(backend);

        Assert.Equal([0, 1], backend.Previewed);

        picker.Question.Apply();
        picker.Question.Apply();

        Assert.Equal([0, 1], backend.PreviewStarts);
    }

    /// <summary>The question renders whenever the draft moves, so being rendered is not what opens a capture.</summary>
    [Fact]
    public void NoScreenIsReadWhileTheDialogIsDown()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var session = new Session(backend, action => action());
        Load(session);

        var picker = Flows.Picker(backend, session);
        picker.Question.SetShowing(true);

        Assert.False(picker.Question.Screens.IsVisible);
        Assert.Empty(picker.Question.Screens.Screens);
        Assert.Empty(backend.Previewed);
        Assert.Empty(backend.PreviewStarts);
    }

    /// <summary>
    /// A picture nobody is looking at goes on grabbing every screen five times a second
    /// for as long as the app is open.
    /// </summary>
    [Fact]
    public void ClosingTheDialogStopsReadingTheScreens()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var picker = Asked(backend);

        Assert.Equal([0, 1], backend.Previewed);

        picker.CancelCommand.Execute(null);

        Assert.Empty(backend.Previewed);
        Assert.False(picker.Question.Screens.IsVisible);
    }

    /// <summary>Dialog and window are separate facts, so either one turning false is enough.</summary>
    [Fact]
    public void AWindowThatWentBehindStopsReadingTheScreens()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var picker = Asked(backend);

        picker.Question.SetShowing(false);

        Assert.Empty(backend.Previewed);
        Assert.False(picker.Question.Screens.IsVisible);
    }

    /// <summary>
    /// Previews outlive the window that asked for them, as decodes do, so nothing else closes one.
    /// A connecting shell learns of the leftover by reading the state,
    /// so that read is on the contract rather than left to the event stream.
    /// </summary>
    [Fact]
    public void AScreenLeftBeingReadByAnEarlierShellIsClosed()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        backend.Previewed.Add(1);

        var session = new Session(backend, action => action());
        Load(session);

        Assert.Equal([1], session.PreviewedMonitors.Select(previewed => previewed.Monitor));

        // Building the question renders once with the dialog down, and that pass closes the leftover.
        Flows.Picker(backend, session);

        Assert.Empty(backend.Previewed);
    }

    /// <summary>Sentence is the backend's own, and nothing is opened where every capture is refused.</summary>
    [Fact]
    public void AMachineThatCannotShowOneScreenSaysSoAndOpensNothing()
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

        var picker = Asked(backend);

        Assert.False(picker.Question.Screens.IsVisible);
        Assert.Empty(backend.Previewed);
        Assert.True(picker.Question.Screens.HasNotice);
        Assert.Contains("Wayland", picker.Question.Screens.Notice);
    }

    /// <summary>
    /// A subscription naming a screen nothing is reading is refused once and never retried,
    /// so a tile made while the start is in flight sits dark for as long as the dialog stands.
    /// </summary>
    [Fact]
    public void AScreenThatIsNotBeingReadYetCarriesNoTile()
    {
        var picker = Asked(new SeededBackend("linux") { AsksWhatToShare = true });

        Assert.All(picker.Question.Screens.Screens, screen => Assert.Null(screen.Tile));
        Assert.All(picker.Question.Screens.Screens, screen => Assert.True(screen.HasPlaceholder));
    }

    [Fact]
    public void AScreenTheBackendIsReadingCarriesATileNamingIt()
    {
        var picker = Asked(Reading(0, 1));
        var tile = picker.Question.Screens.Screens[1].Tile;

        Assert.NotNull(tile);
        Assert.Equal(TileSourceKind.MonitorPreview, tile.Source.Kind);
        Assert.Equal(1, tile.Source.Monitor);
        Assert.False(picker.Question.Screens.Screens[1].HasPlaceholder);
    }

    /// <summary>Form resolves on every keystroke, so a pass rebuilding the tiles re-subscribes that often.</summary>
    [Fact]
    public void ARenderPassKeepsTheTilesItAlreadyHas()
    {
        var picker = Asked(Reading(0, 1));
        var before = picker.Question.Screens.Screens.Select(screen => screen.Tile).ToList();

        Assert.All(before, Assert.NotNull);

        picker.Question.Apply();

        Assert.Equal(before, picker.Question.Screens.Screens.Select(screen => screen.Tile));
    }

    /// <summary>Fixture already reading these screens, which is what puts a tile on a row.</summary>
    private static SeededBackend Reading(params int[] monitors)
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        backend.Previewed.AddRange(monitors);
        return backend;
    }
}
