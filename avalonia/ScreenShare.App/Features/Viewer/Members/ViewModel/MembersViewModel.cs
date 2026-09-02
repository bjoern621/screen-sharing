using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Viewer.Members.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.Members.ViewModel;

/// <summary>
/// Who this machine shares a group with.
///
/// <b>A reading and never a roster kept here.</b> Every member's own app states its presence, the lease lapses
/// where it stops being stated, and a member who left drops out by not appearing,
/// so this renders the last answer whole and merges nothing into it (<c>docs/ipc-api.md</c>, "Events").
///
/// <b>No action of its own.</b> The group key and the name for this machine are what put it in a group,
/// and both are settings, so this card has nothing to press: it says what the group is and what is missing.
/// Nothing in a self-served group removes another member either, so no row affords anything.
///
/// <b>One sentence for a refusal.</b> The presence loop carries whatever the group service or this machine's
/// own settings refused the last statement of presence with, and the card shows that.
/// </summary>
public sealed class MembersViewModel : Observable
{
    public MembersViewModel()
    {
        Rows = [];
        Apply();
    }

    // --- Inputs -------------------------------------------------------------------

    private MembersState? _reported;

    /// <summary>
    /// Group as the presence loop last read it, null before the first read lands.
    /// Written from above on every render pass, so this card holds no copy of the session's state.
    /// </summary>
    public MembersState? Reported
    {
        get => _reported;
        set
        {
            if (Set(ref _reported, value))
            {
                Apply();
            }
        }
    }

    // --- Outputs ------------------------------------------------------------------

    private string _notice = "";
    private bool _hasRows;
    private string _refusal = "";
    private bool _hasRefusal;

    /// <summary>One row per member the group service named, in its order.</summary>
    public ObservableCollection<MemberRow> Rows { get; }

    public string Title => Cards.MembersTitle;

    /// <summary>Why there are no rows, empty while there are some.</summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasRows { get => _hasRows; private set => Set(ref _hasRows, value); }

    /// <summary>Why this machine is outside the group, empty where nothing refused it.</summary>
    public string Refusal { get => _refusal; private set => Set(ref _refusal, value); }

    public bool HasRefusal { get => _hasRefusal; private set => Set(ref _hasRefusal, value); }

    /// <summary>
    /// The one render function.
    /// Stamps the separator onto every row but the last and leaves the bound list alone where nothing differs,
    /// rows being records.
    /// </summary>
    public void Apply()
    {
        var state = Reported;
        var members = state?.Members ?? (IReadOnlyList<Member>)[];

        var rendered = new MemberRow[members.Count];
        for (var i = 0; i < members.Count; i++)
        {
            rendered[i] = MemberRow.Of(members[i]) with { IsLast = i == members.Count - 1 };
        }

        Reconcile.Onto(Rows, rendered);
        HasRows = Rows.Count > 0;

        Notice = HasRows ? ""
            : state is null ? Cards.MembersUnread
            : !state.Joined ? Cards.MembersOutside
            : Cards.MembersNone;

        Refusal = Statements.Of(state?.Refusal);
        HasRefusal = Refusal.Length > 0;

        Assert.That(Rows.Count == members.Count, "a row per member the group named", Rows.Count, members.Count);
        Assert.That(Rows.Count(row => row.IsLast) == (Rows.Count == 0 ? 0 : 1),
            "exactly one row ends the list", Rows.Count);
        Assert.That(HasRows == (Notice.Length == 0),
            "rows and the sentence standing in for them are never both on screen", HasRows);
        Assert.That(HasRefusal == (Refusal.Length > 0), "a refusal and its sentence agree", HasRefusal);
    }
}
