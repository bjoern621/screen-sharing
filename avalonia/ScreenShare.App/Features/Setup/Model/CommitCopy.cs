using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// What the commit calls itself and what it promises, per effect pressing it can have.
///
/// <b>One row per <see cref="PublishCommit"/>, read whole by the view model.</b>
/// A conditional at each binding site would let the label and the sentence under it answer differently
/// (<c>docs/development-principles.md</c>, "Stateless").
///
/// <b>The apply row states the restart in the copy itself.</b>
/// Both engines run a child built from an argv, so new settings tear the pipeline down and launch another,
/// and every viewer loses the picture across the gap.
/// The broadcast screen's quality track is greyed carrying the same reason (<c>Features/Broadcast/Nudge</c>).
///
/// <b>Here rather than in <c>Copy/</c>, which is the layering.</b>
/// Everything in <c>Copy/</c> is keyed on an identifier the backend sends and read by every feature.
/// This is keyed on a state the shell derived, so the table there would point <c>Copy/</c> at a feature
/// and close a cycle.
/// <see cref="PreflightChecks.Clear"/> and <see cref="SetupSteps.ShareLabel"/> sit here for the same reason.
/// </summary>
public static class CommitCopy
{
    /// <summary>
    /// One commit's words.
    /// The promise is split because the stream name is drawn between the halves as an identifier:
    /// the path a viewer asks for, and a reader is owed the chance to see they are about to restart the wrong one.
    /// </summary>
    public sealed record Entry
    {
        public required string Label { get; init; }

        /// <summary>Half in front of the stream name.</summary>
        public required string Lead { get; init; }

        /// <summary>Half after it.</summary>
        public required string Tail { get; init; }
    }

    private static readonly Dictionary<PublishCommit, Entry> Entries = new()
    {
        [PublishCommit.Start] = new Entry
        {
            Label = "Start sharing",
            Lead = "Sharing starts on these settings. Viewers on ",
            Tail = " can watch the stream afterwards.",
        },

        [PublishCommit.Apply] = new Entry
        {
            Label = "Apply and restart",
            Lead = "Applying restarts the stream on these settings. Viewers on ",
            Tail = " lose the picture for a moment and reconnect.",
        },
    };

    /// <summary>
    /// Exhaustive: a commit this table has no row for fails here,
    /// rather than drawing a button labelled after another effect.
    /// </summary>
    public static Entry Of(PublishCommit commit)
    {
        if (!Entries.TryGetValue(commit, out var entry))
        {
            return Assert.Never<Entry>("unexpected commit", (int)commit);
        }

        return entry;
    }
}
