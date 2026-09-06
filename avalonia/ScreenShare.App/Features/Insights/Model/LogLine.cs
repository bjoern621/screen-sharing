using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Insights.Model;

/// <summary>
/// One line of the session log: when, how loud, and what happened.
///
/// The level is the word the log itself prints and not an enum.
/// This screen only renders it, so a level nobody here has seen still reads correctly instead of meeting
/// an exhaustive dispatch it is not in.
///
/// <b>Two producers, reaching this screen by different routes.</b> The event stream carries what this shell did
/// not do, a pipeline that died on its own or a viewer that closed, each with the failure as prose and the run
/// log's path.
/// Audience lines are the difference between two relay rosters, no event announcing a viewer arriving
/// (<c>Audience</c>).
/// The whole log is the file behind <see cref="ExitInfo.LogPath"/>, opened through the backend rather than read
/// here: the file sits on the backend's machine, which this shell's need not be.
/// </summary>
public sealed record LogLine(string Time, string Level, string Message)
{
    private const string Warning = "WARN";

    private const string Info = "INFO";

    /// <summary>The one level drawn loud. Everything quieter stays dim.</summary>
    public bool IsWarning => Level == Warning;

    /// <summary>
    /// Run log this line came from, empty where there is none.
    /// The backend's own path, carried rather than composed: a path built here assumes where the backend keeps
    /// its logs.
    /// </summary>
    public string LogPath { get; init; } = "";

    public bool HasLog => LogPath.Length > 0;

    /// <summary>
    /// One ended child process as a line.
    /// A clean exit carries no message and is worded here rather than printed empty.
    /// A failure is a warning and prints the backend's sentence as it stands, never a paraphrase.
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

    /// <summary>
    /// One viewer arriving or leaving as a line.
    /// Both are ordinary news, the one loud level being kept for a process that failed.
    /// Neither carries a run log: the file is the publisher's, and a viewer elsewhere has no run of this machine's
    /// behind it.
    /// </summary>
    public static LogLine Of(AudienceChange change)
    {
        Assert.NotNull(change, "a log line describes a change that happened");

        return new LogLine(
            change.At.ToLocalTime().ToString(@"HH\:mm\:ss"),
            Info,
            change.Arrived
                ? $"{change.Name} started watching over {change.Via}"
                : $"{change.Name} stopped watching over {change.Via}");
    }
}
