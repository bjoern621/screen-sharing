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
    /// <summary>Raised when the reader asks for the whole log. The card does not own that surface.</summary>
    public event Action? OpenRequested;

    public SessionLogViewModel()
    {
        Lines = [];
        OpenFullLogCommand = new DelegateCommand(() => OpenRequested?.Invoke());
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

    public DelegateCommand OpenFullLogCommand { get; }

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
