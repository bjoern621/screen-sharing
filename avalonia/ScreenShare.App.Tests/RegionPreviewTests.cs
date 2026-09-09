using Avalonia;
using Avalonia.Controls;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.RegionPicker.View;
using ScreenShare.App.Features.Setup.SharePicker.ViewModel;
using ScreenShare.App.Features.Viewer.Tile.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The rectangle is drawn live before the press that sends it, cropped out of the screen holding it.
/// One screen capture, the one the screen grid costs, and the crop is arithmetic on what the catalog reports.
/// </summary>
public sealed class RegionPreviewTests
{
    /// <summary>Rectangle inside the second seeded screen, which sits at 2560,0 and is 1920 by 1080.</summary>
    private const string OnSecondScreen = "2660,300,640x360";

    /// <summary>Dialog up with the window in front, on the kind that names a rectangle.</summary>
    private static SharePickerViewModel Asked(SeededBackend backend)
    {
        var session = new Session(backend, action => action());
        session.Start();
        session.Stop();

        var picker = Flows.Picker(backend, session);
        picker.AskAsync();
        picker.Question.SetShowing(true);

        picker.Question.Group.Fields
            .Single(control => control.Key == ShareLayout.KindKey)
            .Options.Single(option => option.Value == "region")
            .Choose.Execute(null);

        return picker;
    }

    private static void Draw(SharePickerViewModel picker, string region)
        => picker.Question.Group.Fields.Single(control => control.Key == ShareLayout.RegionKey).Text = region;

    [Fact]
    public void TheRectangleIsReadOffTheScreenHoldingIt()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var picker = Asked(backend);

        Draw(picker, OnSecondScreen);
        var region = picker.Question.Region;

        // One screen read, the one the rectangle sits on.
        Assert.Equal([1], backend.Previewed);

        Assert.True(region.HasPreview);
        Assert.Equal(1920, region.SourceWidth);
        Assert.Equal(1080, region.SourceHeight);

        // The rectangle in that screen's own pixels, the setting counting from the desktop's corner.
        Assert.Equal(100, region.CropX);
        Assert.Equal(300, region.CropY);
        Assert.Equal(640, region.CropWidth);
        Assert.Equal(360, region.CropHeight);
    }

    /// <summary>A picture arrives once the backend reports it is reading that screen, and never before.</summary>
    [Fact]
    public void TheCropDrawsTheScreensOwnTile()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        backend.Previewed.Add(1);

        var picker = Asked(backend);
        Draw(picker, OnSecondScreen);
        var tile = picker.Question.Region.Tile;

        Assert.NotNull(tile);
        Assert.Equal(TileSourceKind.MonitorPreview, tile.Source.Kind);
        Assert.Equal(1, tile.Source.Monitor);
    }

    /// <summary>
    /// A rectangle no single screen holds is drawn by nothing:
    /// the capture reads one output at a time, and a crop of the wrong one would be a picture nobody drew.
    /// </summary>
    [Fact]
    public void ARectangleAcrossTwoScreensIsNotDrawn()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var picker = Asked(backend);

        Draw(picker, "2400,300,640x360");

        Assert.False(picker.Question.Region.HasPreview);
        Assert.Null(picker.Question.Region.Tile);
        Assert.Empty(backend.Previewed);
    }

    /// <summary>Nothing drawn is nothing to read, so the rectangle costs a capture only once there is one.</summary>
    [Fact]
    public void NoRectangleReadsNoScreen()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var picker = Asked(backend);

        Assert.False(picker.Question.Region.HasPreview);
        Assert.Empty(backend.Previewed);
    }

    /// <summary>
    /// The widget's half: the screen's picture is scaled until the rectangle fills the box,
    /// and offset so the rectangle's corner lands on the box's.
    /// Everything above rests on this arithmetic, and a box of the wrong shape would show the wrong rectangle.
    /// </summary>
    [Fact]
    public void TheCropScalesTheScreenUntilTheRectangleFillsTheBox()
    {
        var picture = new Border();
        var crop = new RegionCrop
        {
            Child = picture,
            SourceWidth = 1920,
            SourceHeight = 1080,
            CropX = 100,
            CropY = 300,
            CropWidth = 640,
            CropHeight = 360,
        };

        crop.Measure(new Size(288, double.PositiveInfinity));
        crop.Arrange(new Rect(0, 0, 288, crop.DesiredSize.Height));

        // The box takes the rectangle's own shape: 288 wide at 640 by 360 is 162 high.
        Assert.Equal(162, crop.DesiredSize.Height, 3);

        // The screen at 288/640, which is 0.45, with the rectangle's corner pulled onto the box's.
        Assert.Equal(-45, picture.Bounds.X, 3);
        Assert.Equal(-135, picture.Bounds.Y, 3);
        Assert.Equal(864, picture.Bounds.Width, 3);
        Assert.Equal(486, picture.Bounds.Height, 3);
    }

    /// <summary>The picture costs a screen capture, so the dialog closing gives it back.</summary>
    [Fact]
    public void ClosingTheDialogStopsReadingTheScreen()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var picker = Asked(backend);

        Draw(picker, OnSecondScreen);
        Assert.Equal([1], backend.Previewed);

        picker.CancelCommand.Execute(null);

        Assert.Empty(backend.Previewed);
        Assert.False(picker.Question.Region.HasPreview);
    }
}
