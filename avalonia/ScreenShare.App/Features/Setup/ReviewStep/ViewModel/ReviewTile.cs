using ScreenShare.App.Contracts;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.ReviewStep.ViewModel;

/// <summary>
/// One summary tile of the review.
/// Deliberately not key/value rows: the fields are self-describing (<c>latency 120 ms</c>, <c>cq 21</c>), so
/// a key column would print the word twice and halve the width left for the value.
/// A heading, a way back to the step that owns it, and free-form value lines.
///
/// A record whose <see cref="Edit"/> is the owner's own command instance, so two passes over an unchanged
/// review compare equal.
/// </summary>
public sealed record ReviewTile
{
    public required string Heading { get; init; }

    /// <summary>The tile's body, newline-separated. Machine values, in the identifier role.</summary>
    public required string Lines { get; init; }

    /// <summary>Jumps to the step that is the only editor of these values.</summary>
    public required DelegateCommand Edit { get; init; }
}

/// <summary>
/// The tiles: one per group of the resolved form, carrying the shorthand that group settled on and a way back
/// to the step that owns it.
///
/// Composed by the backend rather than here.
/// <c>FieldGroup.summary</c> is the same sentence the strip's chip repeats, so the review and the strip
/// cannot disagree about what a step settled on - which four hand-written tiles of mockup text did,
/// permanently, since they were the same four sentences whatever the settings said.
/// </summary>
public static class ReviewTiles
{
    /// <param name="groups">Each group's key, heading and shorthand, in the form's own order.</param>
    /// <param name="editOf">Hands back a command that moves the flow to the step drawing one group.</param>
    public static IReadOnlyList<ReviewTile> Of(
        IReadOnlyList<(string Key, string Title, string Summary)> groups,
        Func<string, DelegateCommand> editOf)
    {
        Assert.NotNull(groups, "the review summarises the groups the form carried");
        Assert.NotNull(editOf, "a tile needs a command back to the step that owns it");

        var tiles = groups
            .Select(group => new ReviewTile
            {
                Heading = group.Title,
                Lines = group.Summary,
                Edit = editOf(group.Key),
            })
            .ToList();

        Assert.That(tiles.Count == groups.Count, "a tile per group", tiles.Count, groups.Count);
        return tiles;
    }
}
