using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.ViewModel;

namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// Which of the quality group's controls the step lays out, and which fall to the card under it.
///
/// One table for both, the two being a partition.
/// A field in neither is a setting the backend offered that nobody can reach, and a field in both is one setting
/// edited from two places.
///
/// The split is by control kind rather than by key, so a field the form adds to the group lands in whichever half
/// suits its widget with no line changing here.
/// The two exceptions are named because the step draws them as cards and a banded track, which a generic renderer
/// has no shape for, and choosing where a field sits is placement (docs/ipc-api.md, "The rule").
/// </summary>
public static class QualityLayout
{
    /// <summary>
    /// The one group drawn by a layout of its own rather than the generic renderer.
    /// Naming it is placement: what the group holds, what its controls are called and which are greyed stays the
    /// form's answer, and a group the backend renames falls back to the generic renderer.
    /// </summary>
    public const string GroupKey = "quality";

    /// <summary>Rate control, drawn as cards because each option carries a paragraph.</summary>
    public const string ModeKey = "publish.mode";

    /// <summary>Quantizer, drawn on the banded track because its scale has named zones.</summary>
    public const string QuantizerKey = "publish.cq";

    /// <summary>
    /// Whether the step places this control: the two it lays out by name, and every field whose whole control is a
    /// choice, drawn as a dropdown in the read-back row.
    /// A number carrying a ladder is a typed value first, and typed values live in the card below.
    /// </summary>
    public static bool OnStep(FieldViewModel field)
    {
        Assert.NotNull(field, "placing a control needs the control being placed");

        return field.IsSelect || field.IsRadio || field.Key is ModeKey or QuantizerKey;
    }

    /// <summary>Complement of <see cref="OnStep"/>, so between them the group's fields are drawn exactly once.</summary>
    public static bool BelowStep(FieldViewModel field) => !OnStep(field);

    /// <summary>Whether the read-back row draws it: an options field the step does not lay out by name.</summary>
    public static bool InReadbackRow(FieldViewModel field)
        => OnStep(field) && field.Key is not (ModeKey or QuantizerKey);

    /// <summary>
    /// How many cards the rate control sets across, for an option count only the form knows.
    /// Squared off so an option's paragraph gets a third of the column rather than a fifth: five modes across one
    /// row set each explanation thirty characters wide, reading as a column of fragments.
    /// Stated here rather than left to the panel because a UniformGrid asked for neither dimension squares off
    /// <b>both</b>: five options become a three by three, and the row under the cards is a card's worth of empty
    /// column.
    /// Given the columns, the panel divides for the rows and only the last can be short.
    /// </summary>
    public static int CardColumns(int options)
    {
        Assert.That(options >= 0, "a card grid is laid out for a count of options", options);

        var columns = Math.Max(1, (int)Math.Ceiling(Math.Sqrt(options)));
        var rows = (options + columns - 1) / columns;

        Assert.That(rows * columns >= options, "the grid holds every option", options, columns, rows);
        Assert.That((rows - 1) * columns < options, "no row of the grid is empty", options, columns, rows);

        return columns;
    }

    /// <summary>
    /// Where the recommended band starts and ends, as shares of the quantizer scale.
    ///
    /// Shares and not numbers, the scale being the codec's on the engine driving it: 0..51 for the H.26x encoders,
    /// 0..63 for libvpx and software AV1, further for one exposing a raw quantizer index.
    /// A quantizer moves between two scales by proportion, the rule the backend converts a preset's target by
    /// (<c>backend/internal/form/presets.go</c>, <c>presetCq</c>), so one share names the same picture on every
    /// scale.
    /// On the H.26x scale these two read as 18 and 24.
    ///
    /// The track's own colours are four star columns in <c>QualityStepView.axaml</c> holding the same four shares,
    /// a grid column being no element and having no data context to bind through.
    /// </summary>
    public const double QuantizerBandStart = 0.35;

    public const double QuantizerBandEnd = 0.47;

    /// <summary>Number a share of one control's scale lands on, rounded to what the control can hold.</summary>
    public static int QuantizerAt(double minimum, double maximum, double share)
    {
        Assert.That(share >= 0 && share <= 1, "a band edge is a share of the scale", share);
        Assert.That(maximum >= minimum, "a quantizer scale runs upward", minimum, maximum);

        return (int)Math.Round(minimum + (maximum - minimum) * share);
    }
}
