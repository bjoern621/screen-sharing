using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.ViewerTable.ViewModel;

/// <summary>
/// Who is watching, and which of them is having a bad time. The table is the reason a
/// complaint stops being a report and becomes a row: severity is carried by colour,
/// weight and fill, never by a status glyph column that would need its own legend.
///
/// The one thing this render function derives rather than renders: the last row carries
/// no separator, because it sits flush against the card's rounded edge.
/// </summary>
public sealed class ViewerTableViewModel : Observable
{
    public ViewerTableViewModel()
    {
        Rows = [];
        Apply();
    }

    // --- Inputs -------------------------------------------------------------------

    private IReadOnlyList<ViewerRow> _reported = [];
    private int? _readers;

    /// <summary>
    /// What the relay last pushed, in its order. The relay does not know which row ends
    /// the table, so it never sets <see cref="ViewerRow.IsLast"/>.
    ///
    /// Empty on every pass today, for the reason <see cref="Notice"/> states.
    /// </summary>
    public IReadOnlyList<ViewerRow> Reported
    {
        get => _reported;
        set
        {
            Assert.NotNull(value, "a viewer table renders a reported roster");

            if (Set(ref _reported, value))
            {
                Apply();
            }
        }
    }

    /// <summary>
    /// How many readers the relay counts on this stream's path, absent while nothing has been
    /// read or nothing is publishing. It is the whole of what the relay reports about who is
    /// watching, which is why it is stated on its own rather than as a row count.
    /// </summary>
    public int? Readers
    {
        get => _readers;
        set
        {
            if (Set(ref _readers, value))
            {
                Apply();
            }
        }
    }

    // --- Outputs ------------------------------------------------------------------

    private int _strugglingCount;
    private string _summary = "";
    private string _notice = "";
    private bool _hasRows;

    public ObservableCollection<ViewerRow> Rows { get; }

    /// <summary>How many viewers the relay marked struggling. The status bar echoes this count.</summary>
    public int StrugglingCount { get => _strugglingCount; private set => Set(ref _strugglingCount, value); }

    /// <summary>How many are watching, which is the figure the relay does report.</summary>
    public string Summary { get => _summary; private set => Set(ref _summary, value); }

    /// <summary>
    /// Why there are no rows. It names the limit rather than the absence: the relay's snapshot
    /// carries a reader count and no reader identities, and no round trip, loss or buffer fill
    /// is measured anywhere - so an empty table here does not mean nobody is watching.
    /// </summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasRows { get => _hasRows; private set => Set(ref _hasRows, value); }

    /// <summary>
    /// The one render function. Stamps the separator rule onto every row but the last and
    /// leaves the bound list alone when nothing differs - rows are records, so a roster
    /// pushed unchanged every five seconds does not repaint the table under the pointer.
    /// </summary>
    public void Apply()
    {
        var reported = Reported;
        var rendered = new ViewerRow[reported.Count];
        for (var i = 0; i < reported.Count; i++)
        {
            rendered[i] = reported[i] with { IsLast = i == reported.Count - 1 };
        }

        Reconcile.Onto(Rows, rendered);
        StrugglingCount = Rows.Count(row => row.IsStruggling);
        HasRows = Rows.Count > 0;

        Summary = Readers is null
            ? "The relay has not been asked yet."
            : $"{Readers} watching.";
        Notice = HasRows
            ? ""
            : "The relay reports how many are connected and not who they are, and no viewer's "
              + "round trip, loss or buffer fill is measured anywhere, so there is nothing to "
              + "put in a row yet.";

        Assert.That(Rows.Count == reported.Count, "a row per reported viewer", Rows.Count, reported.Count);
        Assert.That(Rows.Count(row => row.IsLast) == (Rows.Count == 0 ? 0 : 1),
            "exactly one row ends the table", Rows.Count);
        Assert.That(HasRows == (Notice.Length == 0), "rows and the sentence standing in for them are never both on screen", HasRows);
    }
}
