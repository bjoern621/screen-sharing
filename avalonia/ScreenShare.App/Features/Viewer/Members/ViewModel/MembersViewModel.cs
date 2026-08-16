using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Viewer.Members.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.Members.ViewModel;

/// <summary>
/// Who this machine shares a group with, and the one control that changes it.
///
/// <b>A reading and never a roster kept here.</b> Every member's own app states its presence, the lease lapses
/// where it stops being stated, and a member who left drops out by not appearing, so this renders the last answer
/// whole and merges nothing into it (<c>docs/ipc-api.md</c>, "Events").
///
/// <b>Two actions and never three.</b> Nothing in a self-served group removes another member, so no row affords
/// anything: what this machine decides is whether it is in the group itself.
/// Join and Leave each name a state, so a press that finds it already true is a success.
///
/// <b>One sentence for a refusal, from whichever side stated it last.</b> The presence loop carries the group
/// service's standing refusal on the state, and a press carries the backend's own; both say why this machine is
/// outside the group, and the press's is the one the reader just caused.
/// A press that went through clears it, so the standing one comes back on the next pass that still has it.
/// </summary>
public sealed class MembersViewModel : Observable
{
    private readonly Action<Action> _dispatch;

    /// <summary>What the last press was refused with, empty while none was.</summary>
    private string _pressRefusal = "";

    /// <param name="dispatch">
    /// Hands work to the UI loop.
    /// An effect answers on whichever thread the transport completed on, and everything written here is read by a
    /// binding that tolerates one thread.
    /// </param>
    public MembersViewModel(IBackend backend, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a member card asks the backend to join and to leave");
        Assert.NotNull(dispatch, "a member card needs a UI loop to marshal an answer back to");

        _dispatch = dispatch;
        Rows = [];

        // Each names the state it wants true, so a press landing on a group this machine is already in or already
        // outside of costs a round trip and changes nothing.
        Join = new PendingCommand(() => PerformAsync(backend.JoinGroupAsync), dispatch, () => CanJoin);
        Leave = new PendingCommand(() => PerformAsync(backend.LeaveGroupAsync), dispatch, () => CanLeave);

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
    private bool _isJoined;
    private bool _canJoin;
    private bool _canLeave;

    /// <summary>One row per member the group service named, in its order.</summary>
    public ObservableCollection<MemberRow> Rows { get; }

    /// <summary>Puts this machine in the group the settings name, and lists it beside everyone else in it.</summary>
    public PendingCommand Join { get; }

    /// <summary>Takes this machine out of the group, which closes what the relay was carrying for it.</summary>
    public PendingCommand Leave { get; }

    /// <summary>Heading over the list.</summary>
    public string Title => Cards.MembersTitle;

    public string JoinLabel => Cards.MembersJoin;

    public string LeaveLabel => Cards.MembersLeave;

    /// <summary>Whether this machine is in the group, which decides which of the two actions is on screen.</summary>
    public bool IsJoined { get => _isJoined; private set => Set(ref _isJoined, value); }

    public bool CanJoin { get => _canJoin; private set => Set(ref _canJoin, value); }

    public bool CanLeave { get => _canLeave; private set => Set(ref _canLeave, value); }

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

        IsJoined = state?.Joined ?? false;
        CanJoin = !IsJoined;
        CanLeave = IsJoined;

        Notice = HasRows ? ""
            : state is null ? Cards.MembersUnread
            : !IsJoined ? Cards.MembersOutside
            : Cards.MembersNone;

        Refusal = _pressRefusal.Length > 0 ? _pressRefusal : Statements.Of(state?.Refusal);
        HasRefusal = Refusal.Length > 0;

        Join.Refresh();
        Leave.Refresh();

        Assert.That(Rows.Count == members.Count, "a row per member the group named", Rows.Count, members.Count);
        Assert.That(Rows.Count(row => row.IsLast) == (Rows.Count == 0 ? 0 : 1),
            "exactly one row ends the list", Rows.Count);
        Assert.That(HasRows == (Notice.Length == 0),
            "rows and the sentence standing in for them are never both on screen", HasRows);
        Assert.That(CanJoin != CanLeave, "one of the two actions is offered and never both", CanJoin, CanLeave);
        Assert.That(HasRefusal == (Refusal.Length > 0), "a refusal and its sentence agree", HasRefusal);
    }

    /// <summary>
    /// Asks for one state of the membership and shows the refusal where there is one.
    /// Nothing is written on the way out: what the group became arrives on the event stream, so the window that
    /// pressed and the window that did not learn it the same way.
    /// </summary>
    private async Task PerformAsync(Func<CancellationToken, Task> effect)
    {
        try
        {
            await effect(default).ConfigureAwait(false);
            Refused("");
        }
        catch (BackendUnavailableException e)
        {
            // Shown as it arrived: the backend names the taken name, which is what makes the refusal actionable.
            Refused(e.Message);
        }
        catch (OperationCanceledException)
        {
        }
    }

    private void Refused(string reason)
    {
        _dispatch(() =>
        {
            _pressRefusal = reason;
            Apply();
        });
    }
}
