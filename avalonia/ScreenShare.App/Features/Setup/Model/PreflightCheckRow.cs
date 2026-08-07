using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
using ScreenShare.App.Controls;

namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// One line of the pre-publish list as the rail and the review render it. A record, so a
/// render pass over an unchanged list compares equal and leaves the bound collection alone.
///
/// <see cref="FixedInStep"/> is what stops an unresolved line from being a dead end: it names
/// the step that owns the control at fault instead of leaving the reader to hunt for it.
/// </summary>
public sealed record PreflightCheckRow
{
    public required string Text { get; init; }

    public required CheckState State { get; init; }

    /// <summary>The step that owns the control this is about, empty where the form named no field.</summary>
    public required string FixedInStep { get; init; }

    public bool IsResolved => State == CheckState.Passed;
}

/// <summary>
/// The list, built from what the form said about the settings as a whole.
///
/// <b>It was two seeded tables and is now neither.</b> The form already answers this question
/// - a diagnostic is one thing worth saying about the settings, ranked by what it costs to
/// ignore - and the seeded lists were a second, fictional answer sitting beside the real one
/// while the real one was printed under the form as loose panels. Both moved here.
///
/// The one thing this side decides is where a diagnostic is anchored: the contract carries the
/// field key it is about, and which step holds that field is placement, which is the shell's
/// (docs/ipc-api.md, "The rule").
/// </summary>
public static class PreflightChecks
{
    /// <summary>
    /// The diagnostics as lines, ranked as the form ranked them.
    /// </summary>
    /// <param name="stepOf">
    /// Names the step that owns one field key, and answers empty for a key no step holds and
    /// for the diagnostics that are about the combination rather than any single field.
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
    /// What the list says when the form found nothing to say. A line rather than an empty
    /// panel: "nothing is wrong" is an answer the reader wants, and a card that vanishes when
    /// the last warning clears reads as a card that broke.
    /// </summary>
    public static readonly PreflightCheckRow Clear = new()
    {
        Text = "Nothing to fix — these settings publish as they stand.",
        State = CheckState.Passed,
        FixedInStep = "",
    };

    /// <summary>
    /// The severity as the list draws it. Exhaustive, so a severity added to the contract
    /// fails here rather than rendering as whatever the default happened to be.
    /// </summary>
    private static CheckState StateOf(Severity severity) => severity switch
    {
        Severity.Error => CheckState.Blocking,
        Severity.Warning => CheckState.Warned,
        Severity.Info => CheckState.Note,
        _ => Assert.Never<CheckState>("unexpected diagnostic severity", (int)severity),
    };

    /// <summary>
    /// The one line a chip can say about the whole list. Derived rather than stored, so the
    /// strip cannot claim two things are owed while the list shows none.
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
