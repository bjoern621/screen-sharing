using System.Globalization;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.StepStrip.ViewModel;

/// <summary>
/// How far the flow has got past one chip. Four states rather than a done flag: the
/// terminal step is neither done nor merely upcoming, and the strip draws it open-ended so
/// nobody reads it as one more form to fill in.
/// </summary>
public enum StepChipState
{
    Done,
    Current,
    Upcoming,
    Terminal,
}

/// <summary>
/// One chip of the strip. A record, so a render pass that changes nothing produces rows
/// that compare equal and the bound collection is left alone -
/// <see cref="Select"/> holds the owner's own command instance for that step, which is
/// what keeps two passes over the same step equal rather than merely equivalent.
/// </summary>
public sealed record StepChipViewModel
{
    /// <summary>The form group this chip's step draws, or the terminal step's key.</summary>
    public required string Key { get; init; }

    public required StepChipState State { get; init; }

    /// <summary>
    /// The step number. A walked step wears a tick instead, and the tick is an icon rather
    /// than a character, so the two cannot be one string: <see cref="IsDone"/> is what says
    /// which of the two the badge is showing.
    /// </summary>
    public required string Badge { get; init; }

    public required string Label { get; init; }

    /// <summary>The value this step settled on. The reason the strip is also the summary.</summary>
    public required string Value { get; init; }

    /// <summary>False on the first chip, which has nothing to its left to join.</summary>
    public required bool HasConnector { get; init; }

    /// <summary>
    /// Whether the connector left of this chip is the bright one. Bright up to and
    /// including the connector leaving the current step: the line reads as the distance
    /// already covered.
    /// </summary>
    public required bool IsConnectorLit { get; init; }

    public required DelegateCommand Select { get; init; }

    public bool IsDone => State == StepChipState.Done;

    public bool IsCurrent => State == StepChipState.Current;

    public bool IsTerminal => State == StepChipState.Terminal;
}

/// <summary>
/// Builds the strip for one set of steps and one current step. Pure and total: the same
/// inputs always yield the same rows, so a render pass calls it unconditionally and the
/// reconcile decides whether anything moved.
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
    /// Current wins over terminal: the last step is drawn open-ended only while the reader
    /// has not reached it, and a white chip that also looked unreachable would read as a
    /// dead end.
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
