using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Insights.TestStreams.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Insights.TestStreams.ViewModel;

/// <summary>
/// Synthetic publishers this machine runs, a row per slot of the set.
///
/// <b>The count and the rows answer different questions.</b> How many are up says nothing about which, so a slot
/// whose child died is readable only from its own row: which slot it is, which relaunch it is on, why it carries
/// no publisher, and the two strings a reader takes elsewhere.
///
/// <b>Nothing here is measured.</b> What a slot is doing is the relay's business and appears on the roster like
/// any other path.
/// This card carries what the backend states about the children it launched.
/// </summary>
public sealed class TestStreamsViewModel : Observable
{
    public TestStreamsViewModel()
    {
        Rows = [];
        Apply();
    }

    // --- Inputs -------------------------------------------------------------------

    private TestStreamState? _reported;

    /// <summary>
    /// Set as the backend last reported it, null before the first read lands.
    /// Written from above on every render pass, so this card holds no copy of the session's state.
    /// </summary>
    public TestStreamState? Reported
    {
        get => _reported;
        set
        {
            if (Set(ref _reported, value))
            {
                Apply();
            }
        }
    }

    // --- Outputs ------------------------------------------------------------------

    private string _summary = "";
    private string _notice = "";
    private bool _hasRows;

    /// <summary>One row per slot the set holds, alive or not, in the backend's order.</summary>
    public ObservableCollection<TestStreamSlotRow> Rows { get; }

    public string Title => Cards.TestStreamsTitle;

    /// <summary>What the card is about, said once over the rows.</summary>
    public string Covers => Cards.TestStreamsCovers;

    /// <summary>
    /// How much of the set is on the air, beside the heading.
    /// The backend's own count against the rows it sent, so a disagreement between the two is visible rather
    /// than smoothed over by counting the rows here.
    /// </summary>
    public string Summary { get => _summary; private set => Set(ref _summary, value); }

    /// <summary>Why there are no rows, empty while there are some.</summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasRows { get => _hasRows; private set => Set(ref _hasRows, value); }

    /// <summary>
    /// The one render function.
    /// Stamps the separator onto every row but the last and leaves the bound list alone where nothing differs,
    /// rows being records.
    /// </summary>
    public void Apply()
    {
        var state = Reported;
        var slots = state?.Slots ?? (IReadOnlyList<TestStreamSlot>)[];

        var rendered = new TestStreamSlotRow[slots.Count];
        for (var i = 0; i < slots.Count; i++)
        {
            rendered[i] = TestStreamSlotRow.Of(slots[i]) with { IsLast = i == slots.Count - 1 };
        }

        Reconcile.Onto(Rows, rendered);
        HasRows = Rows.Count > 0;

        Summary = HasRows ? Cards.TestStreamsRunning(state?.RunningCount ?? 0, Rows.Count) : "";

        Notice = HasRows ? ""
            : state is null ? Cards.TestStreamsUnread
            : Cards.TestStreamsNone;

        Assert.That(Rows.Count == slots.Count, "a row per slot the set holds", Rows.Count, slots.Count);
        Assert.That(Rows.Count(row => row.IsLast) == (Rows.Count == 0 ? 0 : 1),
            "exactly one row ends the list", Rows.Count);
        Assert.That(HasRows == (Notice.Length == 0),
            "rows and the sentence standing in for them are never both on screen", HasRows);
        Assert.That(HasRows == (Summary.Length > 0), "the count describes rows that are on screen", HasRows);
    }
}
