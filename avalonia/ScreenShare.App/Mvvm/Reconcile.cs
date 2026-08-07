using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Mvvm;

/// <summary>
/// Converges a bound collection onto the rows a render function just derived.
///
/// Clear-then-fill rather than an incremental patch: the lists on this screen are small,
/// and clear-then-fill is idempotent by construction. The guard in front of it is what
/// makes that safe to run every pass - rows are records, so an unchanged reading compares
/// equal and the collection is left alone, which is what keeps a per-second refresh from
/// resetting scroll position (docs/development-principles.md, "Idempotency").
/// </summary>
public static class Reconcile
{
    public static void Onto<T>(ObservableCollection<T> bound, IReadOnlyList<T> rows)
    {
        Assert.NotNull(bound, "a reconcile needs the collection it converges");
        Assert.NotNull(rows, "a reconcile needs the rows it converges onto");

        if (bound.SequenceEqual(rows))
        {
            return;
        }

        bound.Clear();
        foreach (var row in rows)
        {
            bound.Add(row);
        }

        Assert.That(bound.Count == rows.Count, "a bound row per rendered row", bound.Count, rows.Count);
    }
}
