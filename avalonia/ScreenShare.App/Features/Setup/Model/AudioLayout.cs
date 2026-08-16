using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.ViewModel;

namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// Which of the audio group's controls sit in a source row, and which are drawn under the list.
///
/// One table for both, the two being a partition: a field in neither is a control the backend offered that
/// nobody can reach, and a field in both is one setting edited from two places.
///
/// The split is by control rather than by entry. Every entry of the list carries the same four, so the copy each
/// one is named and explained by is written once and drawn once, over the column
/// (<c>docs/tooltips.md</c>, "Where the text lives").
/// </summary>
public static class AudioLayout
{
    /// <summary>
    /// The one group drawn as a source list rather than by the generic renderer.
    /// Naming it is placement: what the group holds, what its controls are called and which are greyed stays the
    /// form's answer, and a group the backend renames falls back to the generic renderer.
    /// </summary>
    public const string GroupKey = "audio";

    /// <summary>
    /// The four controls of one entry, as a key with its index taken out
    /// (<see cref="Copy.Fields.Template"/>).
    /// A shell binds "publish.audio_sources[2].gain" and this table names the control that is a value of.
    /// </summary>
    public const string SourceKey = "publish.audio_sources[].source";

    public const string DeviceKey = "publish.audio_sources[].device";

    public const string GainKey = "publish.audio_sources[].gain";

    public const string MuteKey = "publish.audio_sources[].mute";

    /// <summary>Whether a row of the source list draws this control.</summary>
    public static bool InRow(FieldViewModel field)
    {
        Assert.NotNull(field, "placing a control needs the control being placed");

        return InRow(field.Key);
    }

    public static bool InRow(string key)
    {
        Assert.That(key.Length > 0, "placing a control names the control being placed");

        return Copy.Fields.Template(key) is SourceKey or DeviceKey or GainKey or MuteKey;
    }

    /// <summary>Complement of <see cref="InRow(FieldViewModel)"/>, so the group's fields are drawn exactly once.</summary>
    public static bool UnderList(FieldViewModel field) => !InRow(field);

    /// <summary>
    /// Entry a key addresses, -1 for a key addressing no list entry.
    /// The index is the list's own, so the row a reader grows the list by is the one past the last entry the
    /// settings carry (<c>backend/internal/form/form.go</c>, <c>resolveEntries</c>).
    /// </summary>
    public static int EntryOf(string key)
    {
        Assert.That(key.Length > 0, "reading an entry index needs the key it is in");

        var open = key.IndexOf('[');
        var close = key.IndexOf(']');
        if (open < 0 || close < open + 1)
        {
            return -1;
        }

        return int.TryParse(key.AsSpan(open + 1, close - open - 1), out var entry) ? entry : -1;
    }
}
