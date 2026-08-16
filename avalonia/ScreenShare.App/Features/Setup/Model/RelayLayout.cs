namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// The one field key the relay step names, on the licence <see cref="RailLayout"/> takes: naming a key is
/// placement, and what the field is called, what it holds and whether it is reachable stays the form's answer.
///
/// Named because drawing a group key is an effect rather than a value: the service on the relay makes one, and
/// what arrives goes into this box like something that was pasted.
/// A box with no button beside it leaves creating a group to a command line, and a stream published without a key
/// is one anybody may watch, so the two states a reader chooses between are one control apart.
/// </summary>
public static class RelayLayout
{
    /// <summary>The group as the form names it: the relay's address, its listeners and the group key.</summary>
    public const string GroupKey = "relay";

    /// <summary>Secret whose possession is membership of a group.</summary>
    public const string GroupKeyKey = "relay.group_key";
}
