using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Mvvm;
using ScreenShare.Api.V1;

namespace ScreenShare.App.Features.Shell.Update.ViewModel;

/// <summary>
/// What the app says about the release published beside the running build, for both surfaces that say it.
///
/// The status band draws <see cref="Line"/> beside the version and presses <see cref="Check"/>.
/// The dialog behind that line draws the same state at length and presses <see cref="Install"/>.
/// One owner, so the line in the band and the sentence in the dialog cannot disagree.
///
/// Nothing is decided here.
/// Whether this install asks, whether it replaces its own files, and how far a download has got
/// all arrive on <c>UpdateState</c>, and a check reaches every window through the event stream
/// (<c>docs/ipc-api.md</c>).
/// </summary>
public sealed class UpdateViewModel : Observable
{
    private readonly IBackend _backend;
    private readonly Session _session;
    private readonly Action<Action> _dispatch;

    /// <param name="backend">Where the check and the install are asked for.</param>
    /// <param name="session">The one owner of what the backend last said about the release.</param>
    /// <param name="dispatch">Hands a completion back to the UI loop.</param>
    public UpdateViewModel(IBackend backend, Session session, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "an update view asks the backend to check");
        Assert.NotNull(session, "an update view reads what the backend last said");
        Assert.NotNull(dispatch, "an update view marshals completions back to the UI loop");

        _backend = backend;
        _session = session;
        _dispatch = dispatch;

        // Both refuse themselves off the state rather than off a flag of their own,
        // so a control a reader sees enabled is one the backend would accept.
        Check = new PendingCommand(CheckAsync, dispatch, () => CanCheck);
        Install = new PendingCommand(InstallAsync, dispatch, () => CanInstall);
        Open = new DelegateCommand(() => OpenRequested?.Invoke(), () => OpensDialog);
    }

    /// <summary>Asks the backend to read the published release, and to fetch it where it installs one.</summary>
    public PendingCommand Check { get; }

    /// <summary>Starts the staged release, after which the app closes.</summary>
    public PendingCommand Install { get; }

    /// <summary>Asks for the dialog behind the band's line.</summary>
    public DelegateCommand Open { get; }

    /// <summary>
    /// Raised where the reader pressed the band's line and the dialog is to open.
    /// The window's to perform, as a window is the one thing a view model may not raise.
    /// </summary>
    public event Action? OpenRequested;

    /// <summary>
    /// Raised once the backend has the install under way, so the app is to close.
    /// The window's to perform: a view model closing the app would be one deciding the app's lifetime.
    /// </summary>
    public event Action? RestartRequested;

    // --- What the effects landed ---------------------------------------------------

    private string _refused = "";

    /// <summary>
    /// Reads the published release, and fetches it where this install replaces its own files.
    /// The answer travels on the event stream, so nothing here reads a result back.
    /// </summary>
    private async Task CheckAsync()
    {
        try
        {
            await _backend.CheckUpdateAsync().ConfigureAwait(false);
            Landed("");
        }
        catch (BackendUnavailableException e)
        {
            Landed(e.Message);
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <summary>
    /// Starts the staged release and asks the window to close.
    /// The call answers while the app is still up, the applier waiting for it to exit,
    /// so the close follows the reply rather than racing it.
    /// A refusal leaves the app running on the build it had, with the reason on screen.
    /// </summary>
    private async Task InstallAsync()
    {
        try
        {
            await _backend.InstallUpdateAsync().ConfigureAwait(false);
        }
        catch (BackendUnavailableException e)
        {
            Landed(e.Message);
            return;
        }
        catch (OperationCanceledException)
        {
            return;
        }

        Landed("");
        _dispatch(() => RestartRequested?.Invoke());
    }

    private void Landed(string refused)
    {
        _dispatch(() =>
        {
            _refused = refused;
            Apply();
        });
    }

    // --- Outputs -------------------------------------------------------------------

    private string _line = "";
    private bool _showsLine;
    private bool _isFailure;
    private bool _opensDialog;
    private bool _showsPlainLine;
    private bool _canCheck;
    private bool _canInstall;
    private string _checkHint = Updates.Check;
    private string _title = "";
    private string _body = "";
    private string _held = "";
    private bool _showsHeld;
    private string _detail = "";
    private bool _showsDetail;
    private string _pageUrl = "";
    private bool _hasPage;

    /// <summary>One short line for the band, empty where the band says nothing about updates.</summary>
    public string Line { get => _line; private set => Set(ref _line, value); }

    public bool ShowsLine { get => _showsLine; private set => Set(ref _showsLine, value); }

    /// <summary>Whether that line reports a failure, which is drawn in the failure face and selectable.</summary>
    public bool IsFailure { get => _isFailure; private set => Set(ref _isFailure, value); }

    /// <summary>
    /// Whether pressing the line opens the dialog.
    /// A release found is something to act on; a check in flight and an up-to-date build are not.
    /// </summary>
    public bool OpensDialog { get => _opensDialog; private set => Set(ref _opensDialog, value); }

    /// <summary>
    /// Whether the band draws the line as text rather than as a control.
    /// A progress line and a failure are both read rather than pressed,
    /// and a failure is selectable because that is the string a bug report carries.
    /// </summary>
    public bool ShowsPlainLine { get => _showsPlainLine; private set => Set(ref _showsPlainLine, value); }

    /// <summary>Whether the version is a control at all, which an install that asks nothing is not.</summary>
    public bool CanCheck { get => _canCheck; private set => Set(ref _canCheck, value); }

    /// <summary>Whether a staged release is there to install.</summary>
    public bool CanInstall { get => _canInstall; private set => Set(ref _canInstall, value); }

    /// <summary>
    /// What the version says on hover: the offer to check, or why this install checks nothing.
    /// A disabled control leaves a reason rather than a dead end.
    /// </summary>
    public string CheckHint { get => _checkHint; private set => Set(ref _checkHint, value); }

    /// <summary>Dialog heading, naming the release it is about.</summary>
    public string Title { get => _title; private set => Set(ref _title, value); }

    /// <summary>Dialog paragraph: what has happened and what the reader's press does.</summary>
    public string Body { get => _body; private set => Set(ref _body, value); }

    /// <summary>Why this install does not replace its own files, empty where it does.</summary>
    public string Held { get => _held; private set => Set(ref _held, value); }

    public bool ShowsHeld { get => _showsHeld; private set => Set(ref _showsHeld, value); }

    /// <summary>
    /// What the failing side said, drawn as it stands and selectable:
    /// it is the string a reader carries into a bug report.
    /// Carries the backend's refusal of a call as well, that being the same kind of string.
    /// </summary>
    public string Detail { get => _detail; private set => Set(ref _detail, value); }

    public bool ShowsDetail { get => _showsDetail; private set => Set(ref _showsDetail, value); }

    /// <summary>Release page, empty until a check has answered.</summary>
    public string PageUrl { get => _pageUrl; private set => Set(ref _pageUrl, value); }

    public bool HasPage { get => _hasPage; private set => Set(ref _hasPage, value); }

    /// <summary>
    /// One render function.
    /// Every output on every pass, read through the session rather than from anything held here,
    /// so a check another window asked for lands on this one without a write of its own.
    /// The refusal survives every pass: it answers this view's own effect rather than a state.
    /// </summary>
    public void Apply()
    {
        var state = _session.Update;
        var stage = state?.Stage ?? UpdateStage.Unspecified;

        Line = _refused.Length > 0 ? _refused : Updates.Line(state);
        ShowsLine = Line.Length > 0;
        IsFailure = _refused.Length > 0 || stage == UpdateStage.Failed;
        OpensDialog = !IsFailure
            && stage is UpdateStage.Available or UpdateStage.Fetching or UpdateStage.Ready;
        ShowsPlainLine = ShowsLine && !OpensDialog;

        CanCheck = state is not null && stage != UpdateStage.Off;
        CheckHint = stage == UpdateStage.Off ? Statements.Of(state?.Unchecked) : Updates.Check;

        CanInstall = stage == UpdateStage.Ready;

        PageUrl = state?.Page ?? "";
        HasPage = PageUrl.Length > 0;

        Held = Statements.Of(state?.Uninstallable);
        ShowsHeld = Held.Length > 0;

        Detail = _refused.Length > 0 ? _refused : state?.Detail ?? "";
        ShowsDetail = IsFailure && Detail.Length > 0;

        Title = CanInstall
            ? Updates.Title(state?.Latest ?? "")
            : Updates.TitleAvailable(state?.Latest ?? "");

        Body = CanInstall ? Updates.Restart : Updates.ByHand(state?.Running ?? "");

        Check.Refresh();
        Install.Refresh();
        Open.Refresh();

        Assert.That(ShowsLine == (Line.Length > 0), "the band line and the flag drawing it agree", ShowsLine, Line);
        Assert.That(!OpensDialog || ShowsLine, "a line that opens the dialog is a line that is drawn", OpensDialog);
        Assert.That(!CanInstall || CanCheck, "a release only installs where this copy checks at all", CanInstall);
        Assert.That(ShowsHeld == (Held.Length > 0), "the held reason and its text agree", ShowsHeld, Held);
        Assert.That(HasPage == (PageUrl.Length > 0), "the release page and the flag offering it agree", HasPage);
    }
}
