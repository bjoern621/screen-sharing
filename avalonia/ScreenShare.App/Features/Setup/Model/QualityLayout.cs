using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.ViewModel;

namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// Which of the quality group's controls the step lays out and which fall to the drawer
/// beneath it.
///
/// One table for both, because the two are a partition. A field in neither is a setting the
/// backend offered that nobody can reach; a field in both is one setting edited from two
/// places. Stating the split once is what makes those two cases impossible rather than
/// merely unlikely.
///
/// The rule is by control kind rather than by key, which is what keeps it open: a field the
/// form adds to the group lands in whichever of the two suits its widget, with no line
/// changing here. The two exceptions are named because the step draws them as something a
/// generic renderer would not - cards and a banded track - and choosing a place for a field
/// is placement, which is the shell's (docs/ipc-api.md, "The rule").
/// </summary>
public static class QualityLayout
{
    /// <summary>
    /// The one group the flow draws with a layout of its own rather than through the generic
    /// renderer. Naming it is placement and nothing more: what the group contains, what its
    /// controls are called and which of them are greyed is still entirely the form's answer,
    /// and a group the backend renames simply falls back to the generic renderer.
    /// </summary>
    public const string GroupKey = "quality";

    /// <summary>The rate control, drawn as cards because each of its options carries a paragraph.</summary>
    public const string ModeKey = "publish.mode";

    /// <summary>The quantizer, drawn on the banded track because its scale has named zones.</summary>
    public const string QuantizerKey = "publish.cq";

    /// <summary>
    /// Whether the step itself places this control: the two it lays out by name, and every
    /// field whose whole control is a choice, which the read-back row draws as a dropdown.
    /// A number carrying a ladder is not one of those - it is a typed value first, and the
    /// drawer is where typed values live.
    /// </summary>
    public static bool OnStep(FieldViewModel field)
    {
        Assert.NotNull(field, "placing a control needs the control being placed");

        return field.IsSelect || field.IsRadio || field.Key is ModeKey or QuantizerKey;
    }

    /// <summary>
    /// Whether the drawer draws it. The complement of <see cref="OnStep"/>, so between them
    /// the group's fields are drawn exactly once.
    /// </summary>
    public static bool InDrawer(FieldViewModel field) => !OnStep(field);

    /// <summary>
    /// Whether the read-back row draws it: an options field that is not one of the two the
    /// step places itself.
    /// </summary>
    public static bool InReadbackRow(FieldViewModel field)
        => OnStep(field) && field.Key is not (ModeKey or QuantizerKey);

    /// <summary>
    /// How many cards the rate control sets across, for an option count only the form knows.
    ///
    /// The count is squared off so a paragraph gets a third of the column rather than a
    /// fifth: five modes across a single row set each explanation thirty characters wide,
    /// which is read as a column of fragments rather than compared.
    ///
    /// It is stated here rather than left to the panel because a UniformGrid asked for
    /// neither dimension squares off <b>both</b> of them: five options become a three by
    /// three, and the row below the cards is a card's worth of empty column. Given the
    /// columns, the panel divides for the rows and the last one is the only one that can be
    /// short.
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
}
