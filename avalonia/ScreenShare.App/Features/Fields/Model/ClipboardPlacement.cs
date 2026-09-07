namespace ScreenShare.App.Features.Fields.Model;

/// <summary>
/// Which controls carry the button that puts their value on the clipboard.
///
/// Placement, on the licence <see cref="GroupPlacement"/> takes: the contract describes no buttons,
/// and what a field holds stays the form's answer (<c>docs/ipc-api.md</c>, "The rule").
/// Read where a control is rendered rather than handed in per screen,
/// a key worth carrying elsewhere being worth it on every screen that draws it.
///
/// The group key is the offer.
/// It is a secret whose whole use is reaching the people who should watch,
/// and 43 characters of base64 read off a screen arrive wrong.
/// </summary>
public static class ClipboardPlacement
{
    private static readonly HashSet<string> Offers = ["relay.group_key"];

    /// <summary>Keyed on the template, so an indexed key lands with its family.</summary>
    public static bool Offered(string key) => Offers.Contains(Copy.Fields.Template(key));
}
