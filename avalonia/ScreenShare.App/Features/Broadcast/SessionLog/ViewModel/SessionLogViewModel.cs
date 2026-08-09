using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.SessionLog.ViewModel;

/// <summary>
/// What has happened to this session, in the order the log recorded it.
///
/// The card shows a window onto the log and never the whole of it - "Open full log" is
/// the escape hatch, and it navigates rather than expanding in place, so this card cannot
/// grow into a log viewer with its own filters.
/// </summary>
public sealed class SessionLogViewModel : Observable
{
    /// <param name="open">
    /// Opens the whole log, and answers when the backend has. The card does not own that
    /// surface - the files are on the backend's machine - but it does own saying that the
    /// request is out, which is why this is a call it waits on rather than news it raises.
    /// </param>
    /// <param name="dispatch">The UI loop the answer is marshalled back to.</param>
    public SessionLogViewModel(Func<Task> open, Action<Action> dispatch)
    {
        Assert.NotNull(open, "a session log needs somewhere to send a request for the whole of it");
        Assert.NotNull(dispatch, "a session log needs a UI loop to marshal the answer back to");

        Lines = [];
        OpenFullLogCommand = new PendingCommand(open, dispatch);
        Apply();
    }

    // --- Inputs -------------------------------------------------------------------

    private IReadOnlyList<LogLine> _recorded = [];

    /// <summary>
    /// What the backend's event stream has reported this session, newest first. Each line is
    /// one child process that ended, which is what the stream carries about a run: the state
    /// events say what is publishing and these say what happened to what was.
    /// </summary>
    public IReadOnlyList<LogLine> Recorded
    {
        get => _recorded;
        set
        {
            Assert.NotNull(value, "a session log renders recorded lines");

            if (Set(ref _recorded, value))
            {
                Apply();
            }
        }
    }

    // --- Outputs ------------------------------------------------------------------

    private string _notice = "";
    private bool _hasLines;

    public ObservableCollection<LogLine> Lines { get; }

    public PendingCommand OpenFullLogCommand { get; }

    /// <summary>What stands in for the lines while nothing has been reported.</summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasLines { get => _hasLines; private set => Set(ref _hasLines, value); }

    /// <summary>The one render function.</summary>
    public void Apply()
    {
        Reconcile.Onto(Lines, Recorded);
        OpenFullLogCommand.Refresh();

        HasLines = Lines.Count > 0;
        Notice = HasLines ? "" : "Nothing has ended this session. Open the full log for everything before it.";

        Assert.That(Lines.Count == Recorded.Count, "a line per recorded entry", Lines.Count, Recorded.Count);
        Assert.That(HasLines == (Notice.Length == 0), "lines and the sentence standing in for them are never both on screen", HasLines);
    }
}
