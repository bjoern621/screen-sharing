using Avalonia;

namespace ScreenShare.App;

internal static class Program
{
    /// <summary>
    /// STAThread because Windows COM - the clipboard, drag and drop, the file dialogs -
    /// needs the process's first thread to be a single-threaded apartment.
    /// </summary>
    [STAThread]
    public static void Main(string[] args) => BuildAvaloniaApp()
        .StartWithClassicDesktopLifetime(args);

    /// <summary>Also the entry point the XAML previewer and any headless test host call.</summary>
    ///
    /// <remarks>
    /// The windowing backend is detected and then reconsidered, which is the Wayland package's
    /// own contract: it takes the backend already configured as its fallback, so the call has
    /// to follow the detection, and it does nothing at all off Linux.
    ///
    /// It is worth the extra package because of what the fallback costs on a scaled desktop.
    /// Avalonia.Desktop's Linux backend is X11 alone, so a Wayland session runs the window
    /// through XWayland - and a compositor scaling XWayland hands the client the logical size,
    /// then magnifies what it drew. The result is a soft window beside sharp native ones, and
    /// nothing inside the app can correct it: the buffer was already the small one by the time
    /// anything here could look at it. A Wayland client draws at the output's own scale.
    /// </remarks>
    public static AppBuilder BuildAvaloniaApp() => AppBuilder
        .Configure<App>()
        .UsePlatformDetect()
        .UseWaylandWithFallback()
        .WithInterFont()
        .LogToTrace();
}
