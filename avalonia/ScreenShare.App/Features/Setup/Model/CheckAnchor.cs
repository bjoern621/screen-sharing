using ScreenShare.App.Contracts;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// Where one pre-publish line is fixed, and the way there.
/// The list stands beside every step, so a line about a setting the reader cannot see is a dead end
/// until it carries the move to the step drawing that setting.
/// A record, so a pass over an unchanged list compares equal:
/// <see cref="Fix"/> is the strip's own command for the step, one instance per key.
/// </summary>
public sealed record CheckAnchor
{
    /// <summary>What the press says it does: "Go to step 2 · Quality". Empty where the line leads nowhere.</summary>
    public required string Label { get; init; }

    /// <summary>Step's key, which the strip marks its chip from. Empty where the fix is off this flow.</summary>
    public required string StepKey { get; init; }

    /// <summary>Move to where the fix is. Null where the line leads nowhere.</summary>
    public required DelegateCommand? Fix { get; init; }

    public bool HasStep => StepKey.Length > 0;

    /// <summary>
    /// Anchor for a line about the settings as a whole, and for a field no step of this flow draws.
    /// The list still carries it; the strip has nowhere to mark but the terminal chip.
    /// </summary>
    public static readonly CheckAnchor Nowhere = new() { Label = "", StepKey = "", Fix = null };

    /// <summary>
    /// Anchor on a screen outside this flow.
    /// Carries no step key, the strip having no chip for a screen it does not draw,
    /// so a blocking line lands on the terminal chip the way one naming no field does.
    /// </summary>
    public static CheckAnchor Elsewhere(string label, DelegateCommand fix)
    {
        Assert.That(label.Length > 0, "an anchor away from the flow says where it leads");
        Assert.NotNull(fix, "an anchor carries the move to that screen");

        return new CheckAnchor { Label = label, StepKey = "", Fix = fix };
    }

    /// <summary>Anchor on one step of this flow.</summary>
    public static CheckAnchor On(SetupStepRow step, DelegateCommand fix)
    {
        Assert.NotNull(step, "an anchor names the step that fixes the line");
        Assert.NotNull(fix, "an anchor carries the move to that step");

        return new CheckAnchor
        {
            Label = $"Go to step {step.Number} · {step.Label}",
            StepKey = step.Key,
            Fix = fix,
        };
    }
}
