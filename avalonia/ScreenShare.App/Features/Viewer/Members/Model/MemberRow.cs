using Google.Protobuf;
using ScreenShare.Api.V1;
using ScreenShare.App.Copy;

namespace ScreenShare.App.Features.Viewer.Members.Model;

/// <summary>
/// One member of the group, as the card prints it.
///
/// Three facts: what the member goes by,
/// the picture Discord draws them under, and which row is this machine.
/// Who is sending stands on the sharing pill, and who is watching what belongs to the machine
/// doing it, so the card carries neither.
///
/// Record, so a pass over an unchanged answer compares equal and the bound list is left where it is.
/// </summary>
/// <param name="Name">
/// What that member goes by, falling back to its identity where it goes by nothing:
/// a member with no name is still a member, and the id is the string a bug report carries.
/// </param>
/// <param name="Avatar">
/// That member's Discord picture, as the backend read it.
/// Empty outside Discord mode, where a group knows nobody's account, and until a read lands.
/// </param>
/// <param name="IsSelf">This machine's own row, which nothing else on it distinguishes.</param>
/// <param name="IsLast">
/// Ends the list, so the row sits flush against the card's edge and carries no separator.
/// Derived by the render pass: which row ends a list is not a fact the group service has.
/// </param>
/// <param name="ShowsAvatar">
/// Whether the row keeps a column for a picture.
/// Derived by the render pass off the whole list, so every name in a group starts at one edge
/// however many pictures have landed.
/// </param>
public sealed record MemberRow(
    string Name, ByteString Avatar, bool IsSelf, bool IsLast = false, bool ShowsAvatar = false)
{
    /// <summary>Whether a picture landed for this member.</summary>
    public bool HasAvatar => Avatar.Length > 0;

    /// <summary>
    /// What the row says beside the name, empty for anyone but this machine.
    /// In words rather than in a mark, so the list needs no legend beside it.
    /// </summary>
    public string Detail => Cards.MemberDetail(IsSelf);

    public bool HasDetail => Detail.Length > 0;

    public static MemberRow Of(Member member) => new(
        member.DisplayName.Length > 0 ? member.DisplayName : member.MemberId,
        member.Avatar,
        member.Self);
}
