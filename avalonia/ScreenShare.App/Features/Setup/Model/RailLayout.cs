namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// The one field the cost rail names, for two placements of its own.
///
/// Naming a key is placement, which the contract leaves to the shell - the same licence
/// <see cref="QualityLayout"/> uses for the rate control and the quantizer. What the field is
/// called, what it holds, what it may hold and whether it is reachable is still entirely the
/// form's answer.
///
/// The rail <b>reads</b> it beside the bar it is the limit of, because a capacity read three
/// steps away from the prediction it bounds is a number the reader has to remember rather than
/// one they can see. It is a reading and not a control: the rail carried a second spinner over
/// this setting once, which made two controls over one value, so the panel names the step that
/// owns the control instead.
///
/// The measurement is the other placement. It is an effect rather than a value, so it rides
/// beside the control it writes rather than on the panel that reads it
/// (<c>Features/Fields/Model/FieldAction.cs</c>).
/// </summary>
public static class RailLayout
{
    /// <summary>The upload capacity the line is measured against.</summary>
    public const string UplinkKey = "publish.uplink_mbps";
}
