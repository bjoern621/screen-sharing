namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// The one field the cost rail places itself.
///
/// Naming a key is placement, which the contract leaves to the shell - the same licence
/// <see cref="QualityLayout"/> uses for the rate control and the quantizer. What the field is
/// called, what it holds, what it may hold and whether it is reachable is still entirely the
/// form's answer; this says only that the rail draws it beside the bar it is the limit of,
/// because a capacity edited three steps away from the prediction it bounds is a number the
/// reader has to remember rather than one they can see.
///
/// It is the same control the step that owns the field draws, not a second one: both bind the
/// field view model the group holds, so there is one owner and one value.
/// </summary>
public static class RailLayout
{
    /// <summary>The upload capacity the line is measured against.</summary>
    public const string UplinkKey = "publish.uplink_mbps";
}
