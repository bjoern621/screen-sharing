using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
using ScreenShare.App.Controls;

namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// One line of the pre-publish list, as the rail and the review draw it.
/// A record, so a pass over an unchanged list compares equal and leaves the bound collection alone.
/// </summary>
public sealed record PreflightCheckRow
{
    public required string Text { get; init; }

    public required CheckState State { get; init; }

    /// <summary>
    /// Step owning the control at fault, so an unresolved line is not a dead end.
    /// Empty where the form named no field.
    /// </summary>
    public required string FixedInStep { get; init; }

    public bool IsResolved => State == CheckState.Passed;
}

/// <summary>
/// The list, built from the diagnostics the form carried.
/// Every word comes from <c>Copy/</c>, keyed on the statement code the backend sent.
/// The ranking is the form's.
///
/// The one thing decided here is where a line is anchored: the contract names the field a diagnostic is
/// about, and which step holds that field is placement (docs/ipc-api.md, "The rule").
/// </summary>
public static class PreflightChecks
{
    /// <summary>
    /// The diagnostics as lines, in the order the form ranked them.
    /// </summary>
    /// <param name="stepOf">
    /// Names the step owning one field key.
    /// Answers empty for a key no step holds, and for a diagnostic about the combination rather than a field.
    /// </param>
    public static IReadOnlyList<PreflightCheckRow> Of(
        IReadOnlyList<Diagnostic> diagnostics, Func<string, string> stepOf)
    {
        Assert.NotNull(diagnostics, "building the list needs the diagnostics the form carried");
        Assert.NotNull(stepOf, "a line needs somewhere to look up the step that owns its field");

        if (diagnostics.Count == 0)
        {
            return [Clear];
        }

        return diagnostics
            .Select(diagnostic => new PreflightCheckRow
            {
                Text = Copy.Statements.Of(diagnostic.Text),
                State = StateOf(diagnostic.Severity),
                FixedInStep = stepOf(diagnostic.FieldKey),
            })
            .ToList();
    }

    /// <summary>
    /// What the list says when the form found nothing to say, in the sentence
    /// <see cref="Copy.Cards.PreflightClear"/> holds.
    /// </summary>
    public static readonly PreflightCheckRow Clear = new()
    {
        Text = Copy.Cards.PreflightClear,
        State = CheckState.Passed,
        FixedInStep = "",
    };

    /// <summary>
    /// Exhaustive, so a severity added to the contract fails here rather than taking whatever a default arm
    /// would give it.
    /// </summary>
    private static CheckState StateOf(Severity severity) => severity switch
    {
        Severity.Error => CheckState.Blocking,
        Severity.Warning => CheckState.Warned,
        Severity.Info => CheckState.Note,
        _ => Assert.Never<CheckState>("unexpected diagnostic severity", (int)severity),
    };

    /// <summary>
    /// One line about the whole list: "2 blocking", "1 to know about", "nothing to fix".
    /// Derived rather than stored, so the strip cannot claim something is owed while the list shows none.
    /// </summary>
    public static string SummaryOf(IReadOnlyList<PreflightCheckRow> checks)
    {
        Assert.NotNull(checks, "summarising the list needs the list");

        var blocking = checks.Count(check => check.State == CheckState.Blocking);
        if (blocking > 0)
        {
            return blocking == 1 ? "1 blocking" : $"{blocking} blocking";
        }

        var owed = checks.Count(check => !check.IsResolved);
        return owed switch
        {
            0 => "nothing to fix",
            1 => "1 to know about",
            _ => $"{owed} to know about",
        };
    }
}
