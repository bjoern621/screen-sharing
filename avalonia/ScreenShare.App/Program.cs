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
    public static AppBuilder BuildAvaloniaApp() => AppBuilder
        .Configure<App>()
        .UsePlatformDetect()
        .WithInterFont()
        .LogToTrace();
}
