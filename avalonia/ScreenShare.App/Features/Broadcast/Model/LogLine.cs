using ScreenShare.Api.V1;
using ScreenShare.App.Backend;

namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// One line of the session log: when, how loud, and what happened. Mono throughout, because
/// every part of it is machine-generated.
///
/// The level is carried as the word the log itself prints rather than as an enum: this screen
/// only ever renders it, and a level it has never seen still reads correctly instead of
/// falling into an exhaustive-dispatch trap.
///
/// <b>What produces a line is an event, not a poll.</b> The backend's stream carries the
/// changes this shell did not make - a pipeline that died on its own, a viewer that closed,
/// the grid window ending - and each of them arrives with the failure as prose and the run
/// log's path. The card shows those; the whole log is the file behind
/// <see cref="ExitInfo.LogPath"/>, which is opened through the backend rather than read here,
/// because the file is on the backend's machine and this shell may one day not be.
/// </summary>
public sealed record LogLine(string Time, string Level, string Message)
{
    private const string Warning = "WARN";

    private const string Info = "INFO";

    /// <summary>The one level that brightens to white. Everything quieter stays dim.</summary>
    public bool IsWarning => Level == Warning;

    /// <summary>
    /// The run log this line came from, empty where there is none. It is the backend's own path
    /// and is carried rather than parsed: a shell that built one would be assuming where the
    /// backend keeps its logs.
    /// </summary>
    public string LogPath { get; init; } = "";

    public bool HasLog => LogPath.Length > 0;

    /// <summary>
    /// One ended child process as a line. A clean exit carries no message, so it says so
    /// plainly rather than printing an empty one; a failure is a warning and prints the
    /// backend's sentence as it stands.
    /// </summary>
    public static LogLine Of(SessionExit exit)
    {
        var failed = exit.Info.Message.Length > 0;

        return new LogLine(
            exit.At.ToLocalTime().ToString(@"HH\:mm\:ss"),
            failed ? Warning : Info,
            failed ? Figure.Join(exit.What, exit.Info.Message) : Figure.Join(exit.What, "ended cleanly"))
        {
            LogPath = exit.Info.LogPath,
        };
    }
}
