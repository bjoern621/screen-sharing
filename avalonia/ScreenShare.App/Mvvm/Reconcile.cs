using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Mvvm;

/// <summary>
/// Converges a bound collection onto the rows a render function derived.
///
/// Clear-then-fill rather than an incremental patch: these lists are short, and a rebuild leaks no handler and
/// repeats without effect (<c>docs/development-principles.md</c>, "Idempotency").
///
/// The guard in front of it is what makes that safe on every pass.
/// Identity is a row's value at its position, rows being records, so an unchanged reading compares equal and
/// the collection is left untouched.
/// A row that moved therefore rebuilds the list exactly as a row that changed does.
///
/// The guard is correctness and not economy: a rebuild replaces every container, so a per-second pass would
/// reset scroll position, drop a selection, and close a tooltip open under a resting pointer.
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
