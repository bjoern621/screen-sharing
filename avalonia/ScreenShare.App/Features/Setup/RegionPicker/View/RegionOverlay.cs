using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.ApplicationLifetimes;
using Avalonia.Controls.Shapes;
using Avalonia.Platform;
using Avalonia.Input;
using Avalonia.Media;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Setup.RegionPicker.Model;
using ScreenShare.App.Features.Shell.Model;

namespace ScreenShare.App.Features.Setup.RegionPicker.View;

/// <summary>
/// Draws a rectangle on the desktop and answers what was drawn.
///
/// <b>One window per screen.</b> A single window spanning the desktop would be scaled by whichever screen
/// it was placed on, so a drag on a second screen of another scale would land somewhere else.
/// Each window covers one screen at that screen's own scale, and the drag is converted back to the pixels
/// the enumeration counts in.
///
/// <b>The answer is in virtual-desktop pixels</b>, which is what <c>publish.share_region</c> stores:
/// the screen's own origin plus the offset dragged inside it (<c>api/proto/screenshare/v1/settings.proto</c>).
///
/// Empty is a reader who dropped the gesture, on Escape, on a right press, or on a drag with no area in it.
/// </summary>
internal static class RegionOverlay
{
    /// <summary>How the covered screen reads while a rectangle is being drawn.</summary>
    private static readonly IBrush Scrim = new SolidColorBrush(Color.FromArgb(96, 0, 0, 0));

    /// <summary>The rectangle itself: clear inside, so the reader sees what is being framed.</summary>
    private static readonly IBrush Marked = new SolidColorBrush(Color.FromArgb(48, 120, 190, 255));

    private static readonly IBrush MarkedEdge = new SolidColorBrush(Color.FromArgb(230, 150, 210, 255));

    /// <summary>Under this many pixels a drag is a click that missed rather than a rectangle.</summary>
    private const int SmallestSide = 8;

    /// <summary>
    /// Covers every screen, waits for a drag, and answers the rectangle as the settings spell one:
    /// "100,200,1280x720". Empty where nothing was drawn.
    /// </summary>
    public static async Task<string> DrawAsync()
    {
        var screens = Desktop();
        if (screens is null || screens.All.Count == 0)
        {
            return "";
        }

        var answer = new TaskCompletionSource<string>();
        var windows = new List<Window>(screens.All.Count);

        foreach (var screen in screens.All)
        {
            windows.Add(Cover(screen, answer));
        }

        foreach (var window in windows)
        {
            window.Show();
        }

        // The first window takes the keyboard, so Escape reaches a binding without the reader clicking first.
        windows[0].Activate();

        var region = await answer.Task.ConfigureAwait(true);

        foreach (var window in windows)
        {
            window.Close();
        }

        return region;
    }

    /// <summary>
    /// The screens this desktop has, null before a window exists to read them off.
    /// Read through the main window rather than held: a screen plugged in between two presses
    /// is one the next rectangle can be drawn on.
    /// </summary>
    private static Screens? Desktop()
        => (Application.Current?.ApplicationLifetime as IClassicDesktopStyleApplicationLifetime)?.MainWindow?.Screens;

    /// <summary>
    /// One screen's cover: a borderless window at that screen's own bounds, carrying the drag.
    ///
    /// Sized in device-independent units and placed in physical pixels, which is what each property counts in.
    /// It is kept out of this machine's captures like every other window this shell opens, so a monitor preview
    /// running beside the picker does not draw the picker (<see cref="CaptureExclusion"/>).
    /// </summary>
    private static Window Cover(Screen screen, TaskCompletionSource<string> answer)
    {
        var marker = new Rectangle
        {
            Fill = Marked,
            Stroke = MarkedEdge,
            StrokeThickness = 1,
            IsVisible = false,
            HorizontalAlignment = Avalonia.Layout.HorizontalAlignment.Left,
            VerticalAlignment = Avalonia.Layout.VerticalAlignment.Top,
        };

        var window = new Window
        {
            WindowDecorations = WindowDecorations.None,
            WindowStartupLocation = WindowStartupLocation.Manual,
            Topmost = true,
            ShowInTaskbar = false,
            CanResize = false,
            Background = Scrim,
            Position = screen.Bounds.Position,
            Width = screen.Bounds.Width / screen.Scaling,
            Height = screen.Bounds.Height / screen.Scaling,
            Cursor = new Cursor(StandardCursorType.Cross),
            Content = new Canvas { Children = { marker } },
        };

        window.Opened += (_, _) => CaptureExclusions.ForThisSystem().Exclude(window);
        window.KeyDown += (_, key) =>
        {
            if (key.Key == Key.Escape)
            {
                answer.TrySetResult("");
            }
        };

        Track(window, marker, screen, answer);
        return window;
    }

    /// <summary>
    /// The drag itself: press fixes one corner, move draws the rectangle, release answers it.
    ///
    /// A right press drops the gesture, the second way out beside Escape.
    /// A rectangle under <see cref="SmallestSide"/> on either side answers empty as well:
    /// a click that landed on the cover is not a region anybody meant to share.
    /// </summary>
    private static void Track(Window window, Rectangle marker, Screen screen, TaskCompletionSource<string> answer)
    {
        var from = default(Point);
        var dragging = false;

        window.PointerPressed += (_, e) =>
        {
            var point = e.GetCurrentPoint(window);
            if (point.Properties.IsRightButtonPressed)
            {
                answer.TrySetResult("");
                return;
            }

            from = point.Position;
            dragging = true;
            marker.IsVisible = true;
        };

        window.PointerMoved += (_, e) =>
        {
            if (dragging)
            {
                Draw(marker, from, e.GetPosition(window));
            }
        };

        window.PointerReleased += (_, e) =>
        {
            if (!dragging)
            {
                return;
            }

            dragging = false;
            answer.TrySetResult(Region(from, e.GetPosition(window), screen));
        };
    }

    /// <summary>Puts the marker over the two corners, in the window's own units.</summary>
    private static void Draw(Rectangle marker, Point from, Point to)
    {
        marker.Margin = new Thickness(Math.Min(from.X, to.X), Math.Min(from.Y, to.Y), 0, 0);
        marker.Width = Math.Abs(to.X - from.X);
        marker.Height = Math.Abs(to.Y - from.Y);
    }

    /// <summary>
    /// The dragged rectangle in virtual-desktop pixels, and empty where it is too small to be one.
    /// The window's units are device-independent, so each side is taken back to pixels by the screen's scale
    /// before the screen's own origin is added.
    /// </summary>
    private static string Region(Point from, Point to, Screen screen)
    {
        Assert.That(screen.Scaling > 0, "a screen a rectangle is measured on has a scale", screen.Scaling);

        var x = (int)Math.Round(Math.Min(from.X, to.X) * screen.Scaling) + screen.Bounds.X;
        var y = (int)Math.Round(Math.Min(from.Y, to.Y) * screen.Scaling) + screen.Bounds.Y;
        var width = (int)Math.Round(Math.Abs(to.X - from.X) * screen.Scaling);
        var height = (int)Math.Round(Math.Abs(to.Y - from.Y) * screen.Scaling);

        return width < SmallestSide || height < SmallestSide ? "" : ShareRegion.Format(x, y, width, height);
    }
}
