using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Insights.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Insights.SessionLog.ViewModel;

/// <summary>
/// What has happened to this session, in the order the log recorded it.
/// A window onto the log and never the whole of it: "Open full log" navigates rather than expanding in place, so
/// this card cannot grow into a log viewer with filters of its own.
/// </summary>
public sealed class SessionLogViewModel : Observable
{
    /// <param name="open">
    /// Opens the whole log, answering when the backend has.
    /// Awaited rather than raised as news, the card owning the drawing that says the request is out.
    /// </param>
    /// <param name="dispatch">UI loop the answer is marshalled back to.</param>
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
    /// What has happened to this stream, newest first: a child process that ended, or a viewer that started or
    /// stopped watching.
    /// Composed above this card, which renders the lines and decides nothing about what belongs on one.
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

    /// <summary>Stands in for the lines while nothing has been reported.</summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasLines { get => _hasLines; private set => Set(ref _hasLines, value); }

    /// <summary>
    /// The one render function.
    /// The bound lines are rebuilt only where a rendered row differs, so a second pass over one input notifies
    /// nothing.
    /// </summary>
    public void Apply()
    {
        Reconcile.Onto(Lines, Recorded);
        OpenFullLogCommand.Refresh();

        HasLines = Lines.Count > 0;
        Notice = HasLines ? "" : "Nothing has happened this session. Open the full log for everything before it.";

        Assert.That(Lines.Count == Recorded.Count, "a line per recorded entry", Lines.Count, Recorded.Count);
        Assert.That(HasLines == (Notice.Length == 0), "lines and the sentence standing in for them are never both on screen", HasLines);
    }
}
