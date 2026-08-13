using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// One step: the form group it draws, its place in the strip, and what the chip calls it.
/// </summary>
public sealed record SetupStepRow
{
    /// <summary>
    /// The resolved form's group this step draws, or <see cref="SetupSteps.ShareKey"/> on the terminal step,
    /// which draws none.
    /// </summary>
    public required string Key { get; init; }

    /// <summary>
    /// Place in the strip, 1-based.
    /// Also the glyph an unwalked chip's badge wears.
    /// </summary>
    public required int Number { get; init; }

    public required string Label { get; init; }

    /// <summary>The terminal step owns no setting and only asks whether anything blocks the publish.</summary>
    public bool IsTerminal => Key == SetupSteps.ShareKey;
}

/// <summary>
/// The steps of one publish setup, derived from the form the backend answered with.
///
/// <b>The list is not written down here.</b> Which steps exist is which groups the form carries, so a group
/// added to the contract is a step that appears with nothing here to edit, and one renamed leaves no hole
/// (docs/ipc-api.md, "The rule").
///
/// What the shell owns is placement: the order is the form's, one group is drawn by a layout of its own
/// (<see cref="QualityLayout"/>), one is drawn by another destination entirely
/// (<see cref="Fields.Model.GroupPlacement"/>, applied by the caller), and the terminal step is appended
/// because committing is not a group of settings.
/// </summary>
public static class SetupSteps
{
    /// <summary>
    /// The terminal step's key, which is not a group key and never collides with one.
    /// <see cref="For"/> asserts it: a backend growing a group under this name would otherwise produce two
    /// steps claiming one identity.
    /// </summary>
    public const string ShareKey = "share";

    public const string ShareLabel = "Share";

    /// <summary>
    /// The steps for one resolved form: a step per group in the form's order, then the terminal one.
    ///
    /// Empty for a form that has not arrived.
    /// A strip naming steps nothing has described would be the shell asserting a shape it was not told, and
    /// the sentence saying why there is no form is already above the column.
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
                // The backend names the group and this side names the step: the chip has a width the contract
                // cannot see.
                Label = Copy.Fields.Group(group.Key).Title,
            });
        }

        rows.Add(new SetupStepRow { Key = ShareKey, Number = rows.Count + 1, Label = ShareLabel });

        Assert.That(rows.Count == groups.Count + 1, "a step per group, and one to commit", rows.Count, groups.Count);
        return rows;
    }

    /// <summary>
    /// The step this key names, null where the current form has none.
    /// Null is a real answer: the reader can be standing on a step the newest form dropped.
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

    /// <summary>The step after this one, null on the terminal step and on a key the form dropped.</summary>
    public static SetupStepRow? After(IReadOnlyList<SetupStepRow> steps, string key)
    {
        var index = IndexOf(steps, key);
        return index >= 0 && index + 1 < steps.Count ? steps[index + 1] : null;
    }

    /// <summary>The step before this one, null on the first and on a key the form dropped.</summary>
    public static SetupStepRow? Before(IReadOnlyList<SetupStepRow> steps, string key)
    {
        var index = IndexOf(steps, key);
        return index > 0 ? steps[index - 1] : null;
    }
}
