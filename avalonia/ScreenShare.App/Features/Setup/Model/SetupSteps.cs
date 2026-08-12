using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// One step, as a row of facts: which group of the resolved form it draws, its place in the
/// strip, and what the chip calls it.
/// </summary>
public sealed record SetupStepRow
{
    /// <summary>
    /// The resolved form's group this step draws, or <see cref="SetupSteps.ShareKey"/> on the
    /// terminal step, which draws none.
    /// </summary>
    public required string Key { get; init; }

    /// <summary>Its place in the strip, 1-based. Also the glyph an unwalked chip's badge wears.</summary>
    public required int Number { get; init; }

    public required string Label { get; init; }

    /// <summary>The terminal step. It owns no setting; it only asks whether anything blocks the publish.</summary>
    public bool IsTerminal => Key == SetupSteps.ShareKey;
}

/// <summary>
/// The steps of one publish setup, derived from the form the backend answered with.
///
/// <b>The list is not written down here, and that is the fix rather than an omission.</b> It
/// used to be a static table of seven rows, each naming the group key it drew - and three of
/// those keys named groups the backend does not answer with, so three steps of the wizard drew
/// an empty column for as long as nobody clicked them. The four groups the backend does answer
/// with and the table did not name were unreachable at the same time. That is the fork
/// docs/ipc-api.md exists to prevent, in the one place the shell was still allowed to hold a
/// list: which steps exist is which groups the form carries, so a group added to the contract
/// is a step that appears and works with nothing here to edit.
///
/// What the shell still owns is placement, which is what the contract leaves it: the order is
/// the form's, one group is drawn by a layout of its own (<see cref="QualityLayout"/>), one is
/// drawn by another destination entirely (<see cref="Fields.Model.GroupPlacement"/>, and the
/// caller is what applies that filter), and the terminal step is appended because committing is
/// not a group of settings.
/// </summary>
public static class SetupSteps
{
    /// <summary>
    /// The terminal step's key. It is not a group key and must never collide with one, which
    /// <see cref="For"/> asserts rather than assumes - a backend that grew a group called this
    /// would otherwise produce two steps that claim the same identity.
    /// </summary>
    public const string ShareKey = "share";

    public const string ShareLabel = "Share";

    /// <summary>
    /// The steps for one resolved form: a step per group in the form's own order, then the
    /// terminal one.
    ///
    /// Empty for a form that has not arrived. A strip of chips naming steps nothing has
    /// described yet would be the shell asserting a shape it has not been told - and the
    /// sentence saying why there is no form is already above the column.
    /// </summary>
    public static IReadOnlyList<SetupStepRow> For(IReadOnlyList<FieldGroup> groups)
    {
        Assert.NotNull(groups, "building the strip needs the groups the form carried");

        if (groups.Count == 0)
        {
            return [];
        }

        var rows = new List<SetupStepRow>(groups.Count + 1);
        foreach (var group in groups)
        {
            Assert.That(group.Key != ShareKey, "no form group claims the terminal step's key", ShareKey);
            rows.Add(new SetupStepRow
            {
                Key = group.Key,
                Number = rows.Count + 1,
                // The backend names the group and this side names the step: the chip has a
                // width the contract cannot see, so what fits in it is decided here.
                Label = Copy.Fields.Group(group.Key).Title,
            });
        }

        rows.Add(new SetupStepRow { Key = ShareKey, Number = rows.Count + 1, Label = ShareLabel });

        Assert.That(rows.Count == groups.Count + 1, "a step per group, and one to commit", rows.Count, groups.Count);
        return rows;
    }

    /// <summary>
    /// The step this key names, or null where the current form has none. Null is a real
    /// answer: the reader can be standing on a step the newest form dropped.
    /// </summary>
    public static SetupStepRow? Of(IReadOnlyList<SetupStepRow> steps, string key)
    {
        Assert.NotNull(steps, "looking a step up needs the steps to look in");

        foreach (var row in steps)
        {
            if (row.Key == key)
            {
                return row;
            }
        }

        return null;
    }

    public static int IndexOf(IReadOnlyList<SetupStepRow> steps, string key)
    {
        Assert.NotNull(steps, "looking a step up needs the steps to look in");

        for (var i = 0; i < steps.Count; i++)
        {
            if (steps[i].Key == key)
            {
                return i;
            }
        }

        return -1;
    }

    /// <summary>The step after this one, or null on the terminal step and on a key the form dropped.</summary>
    public static SetupStepRow? After(IReadOnlyList<SetupStepRow> steps, string key)
    {
        var index = IndexOf(steps, key);
        return index >= 0 && index + 1 < steps.Count ? steps[index + 1] : null;
    }

    /// <summary>The step before this one, or null on the first and on a key the form dropped.</summary>
    public static SetupStepRow? Before(IReadOnlyList<SetupStepRow> steps, string key)
    {
        var index = IndexOf(steps, key);
        return index > 0 ? steps[index - 1] : null;
    }
}
