using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Controls;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ShareQuestion.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.SharePicker.ViewModel;

/// <summary>
/// The dialog that puts the share question at the press starting a stream.
///
/// <b>The question itself is not this.</b>
/// <see cref="Question"/> holds it, and this is the dialog around it:
/// whether it stands, and what the press waiting on it is told (<see cref="ShareQuestionViewModel"/>).
///
/// <b>A machine whose desktop asks for itself is never asked here.</b>
/// The portal draws the compositor's picker when the capture opens, so its share group resolves with nothing
/// reachable and <see cref="Asks"/> comes false.
/// The press then goes straight to the stream.
///
/// <b>The verdict on the whole draft is drawn here.</b>
/// This dialog is the one way to the question, so the press reaches it on settings that do not publish
/// (<see cref="Model.PublishGate"/>).
/// <see cref="Blocking"/> is then what stands in the way, in the backend's own words,
/// beside a confirm that cannot be pressed.
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

    public SharePickerViewModel(ShareQuestionViewModel question, FormSession form)
    {
        Assert.NotNull(question, "the dialog stands around the share question");
        Assert.NotNull(form, "the dialog reads the backend's verdict on the draft it left");

        Question = question;
        _form = form;

        ConfirmCommand = new DelegateCommand(() => Close(true), () => CanConfirm);
        CancelCommand = new DelegateCommand(() => Close(false), () => IsOpen);

        _form.Changed += Apply;

        Apply();
    }

    // --- Outputs ------------------------------------------------------------------

    private bool _isOpen;
    private bool _canConfirm;

    /// <summary>The question this dialog stands around.</summary>
    public ShareQuestionViewModel Question { get; }

    /// <summary>
    /// What stands between the draft and the air, in the order the form ranked it.
    /// Empty while nothing does.
    /// Blocking lines alone: a warning is something the stream survives, and this list is read
    /// by somebody looking at a confirm that will not press.
    /// </summary>
    public ObservableCollection<PreflightCheckRow> Blocking { get; } = [];

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
    public bool Asks => Question.Asks;

    /// <summary>
    /// Puts the dialog up and answers whether the stream should start.
    /// </summary>
    public Task<bool> AskAsync()
    {
        Assert.That(_answer is null, "one press waits on the picker at a time");

        _answer = new TaskCompletionSource<bool>();
        IsOpen = true;
        Question.SetInDialog(true);
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

        // Anchored nowhere: every screen that fixes one of these is behind this dialog,
        // so a line here says what is wrong and the reader closes the dialog to reach it
        // (Setup/Model/CheckAnchor.cs).
        IReadOnlyList<Diagnostic> diagnostics = form is null ? [] : form.Diagnostics;
        Reconcile.Onto(
            Blocking,
            [.. PreflightChecks.Of(diagnostics, _ => CheckAnchor.Nowhere)
                .Where(check => check.State == CheckState.Blocking)]);

        ConfirmCommand.Refresh();
        CancelCommand.Refresh();

        Assert.That(!CanConfirm || IsOpen, "a closed picker confirms nothing", CanConfirm);
        Assert.That(!CanConfirm || Blocking.Count == 0, "a confirmable draft has nothing blocking it", Blocking.Count);
    }

    /// <summary>
    /// Hands the waiting press its answer and takes the dialog down.
    /// Idempotent: a second call finds nothing waiting and closes a dialog that is already closed.
    /// </summary>
    private void Close(bool share)
    {
        IsOpen = false;
        Question.SetInDialog(false);

        var answer = _answer;
        _answer = null;
        answer?.TrySetResult(share);

        Apply();
    }
}
