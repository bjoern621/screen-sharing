using Avalonia;

namespace ScreenShare.App;

internal static class Program
{
    /// <summary>
    /// STAThread because Windows COM needs the process's first thread to be a single-threaded apartment.
    /// The clipboard, drag and drop and the file dialogs all go through it.
    /// </summary>
    [STAThread]
    public static void Main(string[] args) => BuildAvaloniaApp()
        .StartWithClassicDesktopLifetime(args);

    /// <summary>Entry point for the XAML previewer and any headless test host as well as for Main.</summary>
    ///
    /// <remarks>
    /// UseWaylandWithFallback follows the detection because it takes the backend already configured as its
    /// fallback, and it does nothing at all off Linux.
    /// Avalonia.Desktop's Linux backend is X11 alone, so without it a Wayland session runs the window through
    /// XWayland, and a compositor scaling XWayland hands the client the logical size then magnifies what it
    /// drew.
    /// Nothing inside the app can correct that: the buffer was already the small one by the time anything
    /// here could look at it.
    /// A Wayland client draws at the output's own scale instead.
    ///
    /// EGL before GLX on X11 is the frame channel's requirement rather than a preference.
    /// A decoded frame reaches a tile as a dmabuf descriptor on this platform and the import for one is
    /// EGL's, so a window rendering through GLX draws every tile as a notice saying the frames cannot be
    /// opened (<c>Features/Viewer/Tile/View/DmaBufSurface.cs</c>).
    /// GLX stays behind it: a machine whose EGL will not come up still draws the rest of the app.
    /// </remarks>
    public static AppBuilder BuildAvaloniaApp() => AppBuilder
        .Configure<App>()
        .UsePlatformDetect()
        .With(new X11PlatformOptions
        {
            RenderingMode = [X11RenderingMode.Egl, X11RenderingMode.Glx, X11RenderingMode.Software],
        })
        .UseWaylandWithFallback()
        .WithInterFont()
        .LogToTrace();
}
