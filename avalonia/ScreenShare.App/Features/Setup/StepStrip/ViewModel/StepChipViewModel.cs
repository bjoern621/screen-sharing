using System.Globalization;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.StepStrip.ViewModel;

/// <summary>
/// How far the flow has got past one chip.
/// A walked flag would collapse the terminal step into an upcoming one, and the strip draws that step open-ended
/// so it does not read as another form.
/// </summary>
public enum StepChipState
{
    Done,
    Current,
    Upcoming,
    Terminal,
}

/// <summary>
/// One chip of the strip.
/// A record, so an unchanged pass produces rows that compare equal and the bound collection is left alone.
/// <see cref="Select"/> is the owner's own command instance for that step,
/// which makes two passes over one step equal rather than merely equivalent.
/// </summary>
public sealed record StepChipViewModel
{
    /// <summary>Which form group the step draws, or the terminal step's key, that one drawing none.</summary>
    public required string Key { get; init; }

    public required StepChipState State { get; init; }

    /// <summary>
    /// Step number.
    /// Replaced by a tick once the step is walked, and an icon is not a character, so the two cannot be one string.
    /// <see cref="IsDone"/> says which of them the badge shows.
    /// </summary>
    public required string Badge { get; init; }

    public required string Label { get; init; }

    /// <summary>What this step settled on, which makes the strip the summary as well.</summary>
    public required string Value { get; init; }

    /// <summary>False on the leading chip, which joins nothing to its left.</summary>
    public required bool HasConnector { get; init; }

    /// <summary>
    /// Whether the connector left of this chip is the bright one.
    /// Bright through the connector leaving the current step, so the line reads as ground already covered.
    /// </summary>
    public required bool IsConnectorLit { get; init; }

    public required DelegateCommand Select { get; init; }

    public bool IsDone => State == StepChipState.Done;

    public bool IsCurrent => State == StepChipState.Current;

    public bool IsTerminal => State == StepChipState.Terminal;
}

/// <summary>
/// Builds the strip from one set of steps and the step being stood on.
/// Pure and total, so a render pass calls it unconditionally and the reconcile decides whether anything moved.
/// </summary>
public static class StepChips
{
    public static IReadOnlyList<StepChipViewModel> For(
        IReadOnlyList<SetupStepRow> steps,
        string current,
        Func<SetupStepRow, string> valueOf,
        Func<string, DelegateCommand> commandOf)
    {
        Assert.NotNull(steps, "a strip is built from the steps the form described");
        Assert.NotNull(valueOf, "a chip needs somewhere to read its value from");
        Assert.NotNull(commandOf, "a chip needs a command to carry");

        var currentIndex = SetupSteps.IndexOf(steps, current);
        var chips = new List<StepChipViewModel>(steps.Count);

        for (var i = 0; i < steps.Count; i++)
        {
            var row = steps[i];

            chips.Add(new StepChipViewModel
            {
                Key = row.Key,
                State = StateOf(row, i, currentIndex),
                Badge = row.Number.ToString(CultureInfo.InvariantCulture),
                Label = row.Label,
                Value = valueOf(row),
                HasConnector = i > 0,
                IsConnectorLit = i <= currentIndex + 1,
                Select = commandOf(row.Key),
            });
        }

        Assert.That(chips.Count == steps.Count, "a chip per step", chips.Count, steps.Count);
        Assert.That(
            chips.Count == 0 || chips.Count(chip => chip.IsCurrent) == 1,
            "a strip with steps on it has exactly one current chip", chips.Count(chip => chip.IsCurrent));
        return chips;
    }

    /// <summary>
    /// Current wins over terminal: the last step reads open-ended only until the reader stands on it,
    /// a lit chip that also looked unreachable reading as a dead end.
    /// </summary>
    private static StepChipState StateOf(SetupStepRow row, int index, int currentIndex)
    {
        if (index == currentIndex)
        {
            return StepChipState.Current;
        }

        if (index < currentIndex)
        {
            return StepChipState.Done;
        }

        return row.IsTerminal ? StepChipState.Terminal : StepChipState.Upcoming;
    }
}
