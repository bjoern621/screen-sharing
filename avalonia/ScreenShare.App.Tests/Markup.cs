namespace ScreenShare.App.Tests;

/// <summary>
/// The app's markup, off the checkout the tests were built from.
/// What a view draws with is stated in the file and nowhere else,
/// so a rule about the drawing is read here rather than asked of a control.
/// </summary>
internal static class Markup
{
    /// <summary>Every view in the app, build output left out.</summary>
    public static IEnumerable<string> Views()
    {
        var directory = new DirectoryInfo(AppContext.BaseDirectory);
        while (directory is not null && !File.Exists(Path.Combine(directory.FullName, "ScreenShare.App", "App.axaml")))
        {
            directory = directory.Parent;
        }

        if (directory is null)
        {
            throw new InvalidOperationException("the tests run inside the checkout they were built from");
        }

        var separator = Path.DirectorySeparatorChar;
        return Directory.EnumerateFiles(Path.Combine(directory.FullName, "ScreenShare.App"), "*.axaml",
                SearchOption.AllDirectories)
            .Where(path => !path.Contains($"{separator}obj{separator}") && !path.Contains($"{separator}bin{separator}"));
    }
}
