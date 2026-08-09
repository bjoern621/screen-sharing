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
    /// read or nothing is publishing. It is stated on its own rather than taken as a row count
    /// because the two are different facts about the same answer: the count is what the relay
    /// said, and the rows are what this screen managed to render of it. They agree today - the
    /// backend builds the roster from the array it counts - and stating both is what would make
    /// a day they stopped agreeing visible.
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
    private string _notice = "";
    private bool _hasRows;

    public ObservableCollection<ViewerRow> Rows { get; }

    /// <summary>
    /// How many rows crossed a limit in <see cref="ViewerRow"/>'s severity table.
    ///
    /// Nothing outside this card reads it. The status bar would be the obvious echo and does not
    /// take one: the design draws no figures on the broadcast destination at all, and the band
    /// says nothing there rather than growing one number that this card already shows in colour
    /// (<c>Features/Shell/StatusBar</c>). It is exposed because the count is the card's own
    /// summary of its rows and a test states it, not because a second surface prints it.
    /// </summary>
    public int StrugglingCount { get => _strugglingCount; private set => Set(ref _strugglingCount, value); }

    /// <summary>
    /// Why there are no rows, empty while there are some. It is now only ever the honest reading
    /// of an empty roster - nothing is publishing, or nobody has connected to what is - because
    /// the relay does name its readers. What it no longer says is that the measurement is
    /// missing: a viewer that connects gets a row, so an empty table here means an empty path.
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

        // How many are watching is not stated here. The header pill above this card already
        // carries the count, and one figure written by two render functions is the case that
        // ends with two screens disagreeing. This card is the roster; the count is the header's.

        // Two absences and two sentences, because they leave a publisher with different things
        // to do next: wait for the relay to be asked, or send somebody the link.
        Notice = HasRows ? ""
            : Readers is null ? "The relay has not been asked yet, so there is nobody to list."
            : "Nobody is connected to this stream yet.";

        Assert.That(Rows.Count == reported.Count, "a row per reported viewer", Rows.Count, reported.Count);
        Assert.That(Rows.Count(row => row.IsLast) == (Rows.Count == 0 ? 0 : 1),
            "exactly one row ends the table", Rows.Count);
        Assert.That(HasRows == (Notice.Length == 0), "rows and the sentence standing in for them are never both on screen", HasRows);
    }
}
