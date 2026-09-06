namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// The link a window was started with, read off the arguments the platform handed it.
///
/// The desktop appends it to the command registered for the scheme, so it arrives as an argument like any other
/// (<c>packaging/linux/mirrorme.desktop</c>).
/// Matched by scheme rather than by position: what else rides in argv is the platform's business, and a launcher
/// is free to put its own arguments first.
///
/// What the link means is the backend's answer, which is <c>ResolveLink</c> (<c>backend/internal/applink</c>).
/// This side only picks it out of a list.
/// </summary>
public static class LaunchLink
{
    /// <summary>The scheme this app is registered as the handler for, with its separator.</summary>
    private const string Scheme = "mirrorme:";

    /// <summary>First link among the arguments, empty where none of them is one.</summary>
    public static string In(string[]? args)
    {
        if (args is null)
        {
            return "";
        }

        return args.FirstOrDefault(arg => arg.StartsWith(Scheme, StringComparison.OrdinalIgnoreCase)) ?? "";
    }
}
