using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Setup.ShareStep.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.SharePicker.ViewModel;

/// <summary>
/// The dialog that puts the share question at the press starting a stream.
///
/// <b>The question itself is not this.</b>
/// <see cref="Step"/> holds it, and the wizard's share step draws the same instance,
/// so the reader meets one question with one look wherever it comes up
/// (<see cref="ShareStepViewModel"/>).
/// What is here is the dialog around it: whether it stands, and what the press waiting on it is told.
///
/// <b>A machine whose desktop asks for itself is never asked here.</b>
/// The portal draws the compositor's picker when the capture opens, so its share group resolves with nothing
/// reachable and <see cref="Asks"/> comes false.
/// The press then goes straight to the stream, as it did before.
/// </summary>
public sealed class SharePickerViewModel : Observable
{
    private readonly FormSession _form;

    /// <summary>
    /// The press waiting on this dialog, null while none is.
    /// One at a time: the commit is disabled while a start is in flight,
    /// so a second press cannot reach an open dialog.
    /// </summary>
    private TaskCompletionSource<bool>? _answer;

    public SharePickerViewModel(ShareStepViewModel step, FormSession form)
    {
        Assert.NotNull(step, "the dialog stands around the share question");
        Assert.NotNull(form, "the dialog reads the backend's verdict on the draft it left");

        Step = step;
        _form = form;

        ConfirmCommand = new DelegateCommand(() => Close(true), () => CanConfirm);
        CancelCommand = new DelegateCommand(() => Close(false), () => IsOpen);

        _form.Changed += Apply;

        Apply();
    }

    // --- Outputs ------------------------------------------------------------------

    private bool _isOpen;
    private bool _canConfirm;

    /// <summary>The question, drawn here and on the wizard's own step.</summary>
    public ShareStepViewModel Step { get; }

    /// <summary>Whether the dialog stands over the window.</summary>
    public bool IsOpen { get => _isOpen; private set => Set(ref _isOpen, value); }

    /// <summary>
    /// Whether the draft as it stands can go on the air, which is the backend's own verdict.
    /// A window kind with nothing picked and a region kind with nothing drawn are both refused there,
    /// so the button reads that answer rather than checking the fields itself.
    /// </summary>
    public bool CanConfirm { get => _canConfirm; private set => Set(ref _canConfirm, value); }

    public DelegateCommand ConfirmCommand { get; }

    public DelegateCommand CancelCommand { get; }

    /// <summary>Whether this machine has anything to ask before a stream starts.</summary>
    public bool Asks => Step.Asks;

    /// <summary>
    /// Puts the dialog up and answers whether the stream should start.
    /// </summary>
    public Task<bool> AskAsync()
    {
        Assert.That(_answer is null, "one press waits on the picker at a time");

        _answer = new TaskCompletionSource<bool>();
        IsOpen = true;
        Step.SetInDialog(true);
        Apply();

        return _answer.Task;
    }

    /// <summary>
    /// The one render function.
    /// Safe to run twice: the question renders itself and an unmoved draft asks for no resolve.
    /// </summary>
    public void Apply()
    {
        var form = _form.Form;

        // The backend's verdict on the whole draft, which is what the start would be refused on.
        CanConfirm = IsOpen && (form?.Publishable ?? false) && _form.IsAnswered;

        ConfirmCommand.Refresh();
        CancelCommand.Refresh();

        Assert.That(!CanConfirm || IsOpen, "a closed picker confirms nothing", CanConfirm);
    }

    /// <summary>
    /// Hands the waiting press its answer and takes the dialog down.
    /// Idempotent: a second call finds nothing waiting and closes a dialog that is already closed.
    /// </summary>
    private void Close(bool share)
    {
        IsOpen = false;
        Step.SetInDialog(false);

        var answer = _answer;
        _answer = null;
        answer?.TrySetResult(share);

        Apply();
    }
}
