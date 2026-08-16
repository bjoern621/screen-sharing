namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// The one field key the cost rail names.
///
/// Naming a key is placement, the licence <see cref="QualityLayout"/> uses for the rate control and the
/// quantizer.
/// What the field is called, what it holds, what it may hold and whether it is reachable stays the form's answer.
///
/// The rail <b>reads</b> it beside the bar it is the limit of, a capacity three steps from the prediction it
/// bounds being a number the reader has to remember rather than see.
/// A reading and not a control: a second box here would be two controls over one value, so the panel names the
/// step owning it instead.
///
/// The measurement is an effect rather than a value, so it rides beside that control rather than on the panel
/// reading the figure (<c>Features/Fields/Model/FieldAction.cs</c>).
/// </summary>
public static class RailLayout
{
    /// <summary>Upload capacity the prediction is measured against, Mb/s.</summary>
    public const string UplinkKey = "publish.uplink_mbps";
}
