using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.Model;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.Consent.ViewModel;

/// <summary>
/// The one question an install answers before anything else: whether the log of a crashed run leaves this
/// computer.
///
/// <b>Drawn off the stored answer.</b> The question stands while the settings say it has not been put, and
/// the write recording it is what takes the dialog down
/// (<c>backend/internal/settings/app.go</c>, <c>CrashReportsAsked</c>).
/// So the state closing it is the state the next start reads, and nothing here remembers having asked.
///
/// <b>The toggle is the app group's own field</b>, drawn by the renderer every other screen shares and
/// stored the way every applied setting is, so this dialog and the settings dialog write one message
/// (<c>docs/settings-editing.md</c>, "Staged and applied").
///
/// <b>A backend holds the send back until this is answered</b>, so the question decides something rather
/// than announcing a default (<c>backend/internal/app/report.go</c>).
///
/// <b>Outputs</b> only, written by <see cref="Apply"/> on every pass.
/// </summary>
public sealed class ConsentViewModel : Observable
{
    /// <summary>The toggle the answer is given on, placed by name under the paragraph.</summary>
    private const string CrashReportsKey = "app.send_crash_reports";

    /// <summary>
    /// Where the asking is recorded. No control writes it, so it leaves through the write addressing a field
    /// the form draws none for (<see cref="FormSession.WriteUndrawn"/>).
    /// </summary>
    private const string AskedKey = "app.crash_reports_asked";

    private readonly FormSession _form;
    private readonly Session _session;

    /// <param name="form">Draft this window holds, and where the answer leaves through.</param>
    /// <param name="session">Vocabulary naming what the control offers.</param>
    public ConsentViewModel(FormSession form, Session session)
    {
        Assert.NotNull(form, "the question draws the draft the window is holding");
        Assert.NotNull(session, "the question reads the vocabulary naming what its control offers");

        _form = form;
        _session = session;

        Group = new FieldGroupViewModel(_form.Write);

        CloseCommand = new DelegateCommand(Answer);

        // News that the draft, or the form behind it, moved. The answer this dialog writes arrives on it too.
        _form.Changed += Apply;

        Apply();
    }

    // --- Outputs ------------------------------------------------------------------

    private bool _isOpen;
    private FieldViewModel? _crashReports;

    /// <summary>Whether the question stands over the window.</summary>
    public bool IsOpen { get => _isOpen; private set => Set(ref _isOpen, value); }

    /// <summary>The app group of the resolved form, holding the toggle below and its write path.</summary>
    public FieldGroupViewModel Group { get; }

    /// <summary>The toggle the answer is given on, null until a form carries it.</summary>
    public FieldViewModel? CrashReports { get => _crashReports; private set => Set(ref _crashReports, value); }

    /// <summary>Records that the question was put, which is what closes it.</summary>
    public DelegateCommand CloseCommand { get; }

    /// <summary>
    /// The one render function.
    /// Safe to run twice: an unmoved draft asks for no resolve,
    /// and the group's own pass produces a field that compares equal.
    /// </summary>
    public void Apply()
    {
        _form.Sync();

        var form = _form.Form;
        Group.Apply(GroupOf(form), _session.Words, form?.Settings, _form.IsAnswered);
        CrashReports = Group.Visible(CrashReportsKey);

        // A form that carries no toggle leaves nothing to answer on, so the question waits for one.
        IsOpen = CrashReports is not null && _form.Draft is { App.CrashReportsAsked: false };

        Assert.That(
            _form.Draft is not { App.CrashReportsAsked: true } || !IsOpen,
            "a question the settings record as put stands over nothing");
    }

    /// <summary>
    /// Names the state the press asks for, that the question has been put, so a second press asks for what
    /// already holds (<c>docs/development-principles.md</c>, "Idempotency").
    ///
    /// The toggle's own value is stored as it is moved, the group being applied,
    /// so this write records the asking alone.
    /// </summary>
    private void Answer()
    {
        if (_form.Draft is not { App.CrashReportsAsked: false })
        {
            return;
        }

        _form.WriteUndrawn(AskedKey, new Api.V1.FieldValue { Flag = true });
    }

    /// <summary>
    /// App group of the resolved form, null where the form carries none.
    /// Found by the key deciding which surface draws a group, so this question and the settings dialog read one
    /// table (<see cref="GroupPlacement"/>).
    /// </summary>
    private static Api.V1.FieldGroup? GroupOf(Api.V1.Form? form)
        => form?.Groups.FirstOrDefault(group => GroupPlacement.InAppSettings(group.Key));
}
