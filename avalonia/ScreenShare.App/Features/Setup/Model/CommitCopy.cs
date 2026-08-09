using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// What the commit calls itself, and what it promises, for each of the two things pressing it
/// can do.
///
/// <b>One table, read by the view model.</b> The label and the sentence under it move together
/// - a button that said "Go live" over a paragraph about restarting would be two answers to one
/// question - so they are one row per <see cref="PublishCommit"/> rather than two conditionals
/// at two binding sites (<c>docs/development-principles.md</c>, "Declarative").
///
/// <b>The apply row says what applying costs, in the copy itself.</b> The backend has no
/// live-safe change: both engines run a child built from an argv, so putting new settings on a
/// running stream tears the pipeline down and launches another, and every viewer loses the
/// picture across the gap. A reader who presses this expecting a seamless change has been
/// misled by the button, so the button says otherwise - the same treatment the broadcast
/// screen's quality track gets, which is greyed and carries the same reason
/// (<c>Features/Broadcast/Nudge</c>).
///
/// <b>It is here rather than in <c>Copy/</c>, and that is the layering rather than an
/// oversight.</b> Everything in <c>Copy/</c> is keyed on an identifier the backend sends - a
/// field key, a value, a statement code - and every feature reads it. This is keyed on a state
/// this module derived, so putting the table there would make <c>Copy/</c> depend on a feature
/// and close a cycle. The same reasoning already keeps <see cref="PreflightChecks.Clear"/> and
/// <see cref="SetupSteps.GoLiveLabel"/> here.
/// </summary>
public static class CommitCopy
{
    /// <summary>
    /// One commit's words: the label on the button, and the promise under it.
    ///
    /// The promise is two halves because the stream name sits inside it at full strength. It is
    /// the identifier the promise is about - the path a viewer will really ask for - and a
    /// reader is owed the chance to notice they are about to restart the wrong one.
    /// </summary>
    public sealed record Entry
    {
        /// <summary>What the button says it will do.</summary>
        public required string Label { get; init; }

        /// <summary>The half of the promise in front of the stream name.</summary>
        public required string Lead { get; init; }

        /// <summary>The half after it.</summary>
        public required string Tail { get; init; }
    }

    private static readonly Dictionary<PublishCommit, Entry> Entries = new()
    {
        [PublishCommit.Start] = new Entry
        {
            Label = "Go live",
            Lead = "Going live starts sending immediately. Viewers on ",
            Tail = " will see the stream within about two seconds.",
        },

        [PublishCommit.Apply] = new Entry
        {
            Label = "Apply and restart",
            Lead = "Applying restarts the stream. The encoder is torn down and launched again on these settings - "
                + "there is no live-safe change here and never was - so viewers on ",
            Tail = " lose the picture for a moment and reconnect.",
        },
    };

    /// <summary>
    /// The words for one commit. Exhaustive: a commit the gate can name and this table cannot
    /// fails here rather than drawing a button labelled after the other one.
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
