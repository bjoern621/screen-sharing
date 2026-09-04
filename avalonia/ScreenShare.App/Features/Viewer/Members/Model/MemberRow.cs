using ScreenShare.Api.V1;
using ScreenShare.App.Copy;

namespace ScreenShare.App.Features.Viewer.Members.Model;

/// <summary>
/// One member of the group, as the card prints it.
///
/// Two facts: what the member goes by, and which row is this machine.
/// Who is sending stands on the sharing pill, and who is watching what belongs to the machine
/// doing it, so the card carries neither.
///
/// Record, so a pass over an unchanged answer compares equal and the bound list is left where it is.
/// </summary>
/// <param name="Name">
/// What that member goes by, falling back to its identity where it goes by nothing:
/// a member with no name is still a member, and the id is the string a bug report carries.
/// </param>
/// <param name="IsSelf">This machine's own row, which nothing else on it distinguishes.</param>
/// <param name="IsLast">
/// Ends the list, so the row sits flush against the card's edge and carries no separator.
/// Derived by the render pass: which row ends a list is not a fact the group service has.
/// </param>
public sealed record MemberRow(string Name, bool IsSelf, bool IsLast = false)
{
    /// <summary>
    /// What the row says beside the name, empty for anyone but this machine.
    /// In words rather than in a mark, so the list needs no legend beside it.
    /// </summary>
    public string Detail => Cards.MemberDetail(IsSelf);

    public bool HasDetail => Detail.Length > 0;

    public static MemberRow Of(Member member) => new(
        member.DisplayName.Length > 0 ? member.DisplayName : member.MemberId,
        member.Self);
}
