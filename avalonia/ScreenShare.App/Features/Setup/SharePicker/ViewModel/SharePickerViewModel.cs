using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.SharePicker.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.SharePicker.ViewModel;

/// <summary>
/// What the stream shares, asked at the press that starts it.
///
/// <b>The dialog draws one group of the resolved form.</b>
/// Which controls it holds, which of them are reachable and what a grayed one says
/// are the backend's answers, arriving in the share group (<c>docs/ipc-api.md</c>, "The rule").
/// Placement is this side's:
/// the group is drawn here and on the wizard's own step, one group on two screens.
///
/// <b>A machine whose desktop asks for itself is never asked here.</b> The portal draws the compositor's
/// picker when the capture opens, so its share group resolves with nothing reachable and
/// <see cref="Asks"/> comes false. The press then goes straight to the stream, as it did before.
///
/// <b>The window list is read when the dialog opens.</b> Windows open and close while nobody is looking,
/// so a list held across presses would offer one that is gone (<c>ListShareWindows</c>).
///
/// <b>Outputs</b> only, written by <see cref="Apply"/> on every pass. The answer leaves through the draft:
/// the controls write it where every other control does, and the press that opened this reads it back.
/// </summary>
public sealed class SharePickerViewModel : Observable
{
    /// <summary>Group of the resolved form this dialog draws, as the backend names it.</summary>
    public const string GroupKey = "share";

    /// <summary>The one control this dialog fills itself, a rectangle being dragged rather than typed.</summary>
    private const string RegionKey = "publish.share_region";

    private readonly IBackend _backend;
    private readonly FormSession _form;
    private readonly Session _session;
    private readonly Action<Action> _dispatch;

    /// <summary>
    /// Draws a rectangle on the screen and answers what was drawn, empty where the reader dropped it.
    /// Injected because a rectangle is drawn in a window over the desktop, which a view model does not open
    /// (<c>avalonia/README.md</c>).
    /// </summary>
    private readonly Func<Task<string>> _drawRegion;

    /// <summary>
    /// The press waiting on this dialog, null while none is.
    /// One at a time: the commit is disabled while a start is in flight,
    /// so a second press cannot reach an open dialog.
    /// </summary>
    private TaskCompletionSource<bool>? _answer;

    /// <summary>Windows as they stood when the dialog opened, so their titles name the handles offered.</summary>
    private IReadOnlyList<ShareWindow> _windows = [];

    public SharePickerViewModel(
        IBackend backend, FormSession form, Session session, Func<Task<string>> drawRegion, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "the picker reads the windows this machine has open");
        Assert.NotNull(form, "the picker draws the draft the window is holding");
        Assert.NotNull(session, "the picker names its entries out of what the backend answered");
        Assert.NotNull(drawRegion, "the picker needs somewhere to draw a rectangle");
        Assert.NotNull(dispatch, "the picker marshals an answer back to the UI loop");

        _backend = backend;
        _form = form;
        _session = session;
        _drawRegion = drawRegion;
        _dispatch = dispatch;

        Group = new FieldGroupViewModel(_form.Write);

        ConfirmCommand = new DelegateCommand(() => Close(true), () => CanConfirm);
        CancelCommand = new DelegateCommand(() => Close(false), () => IsOpen);
        DrawRegionCommand = new PendingCommand(DrawRegionAsync, dispatch, () => IsOpen);

        _form.Changed += Apply;

        Apply();
    }

    // --- Outputs ------------------------------------------------------------------

    private bool _isOpen;
    private bool _canConfirm;
    private bool _drawsRegion;

    /// <summary>One group of the form, rendered by the generic renderer every other group uses.</summary>
    public FieldGroupViewModel Group { get; }

    /// <summary>Whether the dialog stands over the window.</summary>
    public bool IsOpen { get => _isOpen; private set => Set(ref _isOpen, value); }

    /// <summary>
    /// Whether the draft as it stands can go on the air, which is the backend's own verdict.
    /// A window kind with nothing picked and a region kind with nothing drawn are both refused there,
    /// so the button reads that answer rather than checking the fields itself.
    /// </summary>
    public bool CanConfirm { get => _canConfirm; private set => Set(ref _canConfirm, value); }

    /// <summary>
    /// Whether a rectangle can be drawn, which is whether the region control is on screen.
    /// The backend draws that control under the region kind alone, so the button follows it
    /// rather than reading the kind itself.
    /// </summary>
    public bool DrawsRegion { get => _drawsRegion; private set => Set(ref _drawsRegion, value); }

    public DelegateCommand ConfirmCommand { get; }

    public DelegateCommand CancelCommand { get; }

    public PendingCommand DrawRegionCommand { get; }

    /// <summary>
    /// Whether this machine has anything to ask before a stream starts.
    /// False where the share group carries no control the reader can move:
    /// the desktop's own picker answers there, and a dialog offering nothing would stand between
    /// the press and the stream for no reason.
    /// </summary>
    public bool Asks
    {
        get
        {
            var group = GroupOf(_form.Form);
            if (group is null)
            {
                return false;
            }

            foreach (var control in group.Fields)
            {
                if (control.Visible && control.Enabled)
                {
                    return true;
                }
            }

            return false;
        }
    }

    /// <summary>
    /// Puts the dialog up and answers whether the stream should start.
    ///
    /// The window list is read first, so the handles the control offers are named by what is open now.
    /// A read that fails leaves the list empty: the control still offers whatever the form does,
    /// under the handles the backend sent, which is a picker that works and reads badly rather than none.
    /// </summary>
    public async Task<bool> AskAsync()
    {
        Assert.That(_answer is null, "one press waits on the picker at a time");

        try
        {
            _windows = await _backend.ShareWindowsAsync().ConfigureAwait(true);
        }
        catch (BackendUnavailableException)
        {
            _windows = [];
        }

        _answer = new TaskCompletionSource<bool>();
        IsOpen = true;
        Apply();

        return await _answer.Task.ConfigureAwait(true);
    }

    /// <summary>
    /// The one render function.
    /// Safe to run twice: the group's own pass produces fields that compare equal,
    /// and an unmoved draft asks for no resolve.
    /// </summary>
    public void Apply()
    {
        _form.Sync();

        var form = _form.Form;
        Group.Apply(GroupOf(form), _session.Words.With(_windows), form?.Settings, _form.IsAnswered);

        // The backend's verdict on the whole draft, which is what the start would be refused on.
        CanConfirm = IsOpen && (form?.Publishable ?? false) && _form.IsAnswered;
        DrawsRegion = Group.Visible(RegionKey) is not null;

        ConfirmCommand.Refresh();
        CancelCommand.Refresh();
        DrawRegionCommand.Refresh();

        Assert.That(!CanConfirm || IsOpen, "a closed picker confirms nothing", CanConfirm);
    }

    /// <summary>
    /// Hands the waiting press its answer and takes the dialog down.
    /// Idempotent: a second call finds nothing waiting and closes a dialog that is already closed.
    /// </summary>
    private void Close(bool share)
    {
        IsOpen = false;

        var answer = _answer;
        _answer = null;
        answer?.TrySetResult(share);

        Apply();
    }

    /// <summary>
    /// Draws a rectangle over the desktop and writes it into the draft.
    ///
    /// A rectangle that came back empty is a reader who dropped the gesture, and it writes nothing:
    /// the draft keeps whatever it held, so backing out of the overlay leaves the last region standing.
    /// </summary>
    private async Task DrawRegionAsync()
    {
        var region = await _drawRegion().ConfigureAwait(true);
        if (region.Length == 0)
        {
            return;
        }

        _dispatch(() =>
        {
            _form.Write(RegionKey, new FieldValue { Text = region });
            Apply();
        });
    }

    private static FieldGroup? GroupOf(Form? form)
    {
        if (form is null)
        {
            return null;
        }

        foreach (var group in form.Groups)
        {
            if (group.Key == GroupKey)
            {
                return group;
            }
        }

        return null;
    }
}
