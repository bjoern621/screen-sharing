using Avalonia.Threading;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Features.Broadcast.ViewModel;
using ScreenShare.App.Features.Setup.ViewModel;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.NavStrip.ViewModel;
using ScreenShare.App.Features.Shell.StatusBar.ViewModel;
using ScreenShare.App.Features.Shell.TitleBar.ViewModel;
using ScreenShare.App.Features.Viewer.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.ViewModel;

/// <summary>
/// The window's own state. It owns the two facts everything else on screen turns on -
/// which destination is showing, and whether broadcast can be reached at all - and it is
/// the only owner: the three chrome bands hold no destination of their own, they are told
/// one on every render pass, which is what makes a dimmed segment disagreeing with the body
/// impossible rather than merely unlikely.
///
/// <see cref="Show"/> and <see cref="SetBroadcastAvailable"/> are the named writes.
/// <see cref="Apply"/> is the one render function: it pushes the state into every child and
/// picks the body on every pass, including the branches that turn something off
/// (docs/development-principles.md, "One render function per component").
/// </summary>
public sealed class ShellViewModel : Observable
{
    /// <summary>
    /// What the title bar calls the window before anything is publishing. Once a stream is in
    /// force the window is named after it, which is the backend's own stream name rather than
    /// one composed here.
    /// </summary>
    private const string Idle = "no stream";

    /// <summary>
    /// Opens on the viewer, live. That is the one state the design specifies end to end -
    /// title, breadcrumb, pill and every status-bar segment - so the window renders from
    /// verbatim copy rather than from anything this module made up.
    /// </summary>
    private Destination _current = Destination.Viewer;

    /// <summary>
    /// The running state, owned once for the whole window and read through by everything that
    /// draws from it.
    /// </summary>
    private readonly Session _session;

    private bool _broadcastAvailable;

    /// <summary>
    /// Whether the reader is owed the broadcast screen: they asked for it on the review and the
    /// stream they started has not landed yet.
    ///
    /// It is a flag rather than a straight navigation because a start that was accepted is not
    /// yet a stream in force - the reply carries nothing and the live state arrives on the event
    /// stream a moment later. Moving on the reply would be the window claiming a state the
    /// backend has not reported, which <see cref="Show"/> refuses outright.
    /// </summary>
    private bool _opensBroadcast;

    private object _body;

    public ShellViewModel()
    {
        TitleBar = new TitleBarViewModel();
        Nav = new NavStripViewModel(Show);
        StatusBar = new StatusBarViewModel();

        // The Go control plane, over the local socket. The window constructs it here because
        // the shell owns the backend connection, and every value, label, greying and figure the
        // three destinations draw arrives through it already decided (docs/ipc-api.md).
        //
        // Constructing it opens nothing: the connection is made by the first read and remade
        // by the next one after a failure, so a window whose backend is not running yet paints
        // its notice and reaches the backend when there is one.
        //
        // The dispatcher beside it is the other half. The seam's reads are asynchronous
        // because a socket is, so their answers land on whichever thread the transport
        // completed on and have to be marshalled back before a bound property is written.
        // Posted rather than checked first, because a post from the UI thread is already the
        // right thing.
        var backend = new ControlBackend();
        var dispatch = (Action<Action>)(action => Dispatcher.UIThread.Post(action));

        // One session for the whole window, and the one owner of the running state. Broadcast
        // and the viewer describe the same session from two angles, so a session each would be
        // two windows' worth of reads and two chances to disagree about what is publishing.
        _session = new Session(backend, dispatch);

        Setup = new SetupViewModel(backend, _session, dispatch);
        Broadcast = new BroadcastViewModel(backend, _session, dispatch);
        Viewer = new ViewerViewModel(backend, _session, dispatch);

        // Every destination re-renders on any change, because the chrome reads all three: the
        // nav strip dims broadcast when nothing publishes and the status band prints the
        // viewer's figures whichever destination is showing.
        _session.Changed += OnSessionChanged;

        _body = BodyFor(_current);

        // The status band prints figures the viewer owns, and a reader toggling a chip
        // changes them without going through any write of the shell's, so the band is
        // re-rendered from the viewer's own notification rather than left stale until the
        // next destination change.
        Viewer.PropertyChanged += (_, _) => RenderStatusBar();

        // "Edit in setup" is the entire fix for setup and broadcast both owning
        // configuration: the live screen names the request and the shell performs it, so the
        // control that edits a value still lives in exactly one window. Every other action the
        // screen raises is performed by the screen itself against the backend.
        Broadcast.ActionRequested += OnBroadcastRequested;

        // The other half of the same rule: setup owns the commit and performs it, and where the
        // window goes afterwards is the window's. It is a request rather than a navigation
        // because the stream is not in force until the backend says it is.
        Setup.WentLive += OnWentLive;

        // Rendered before anything is read, so the window has a complete view model to paint
        // whether or not the backend is reachable, and the first state is a later pass rather
        // than a precondition of the first one.
        Apply();

        // Started after that first pass, and not awaited: it reads four states and then holds
        // the event stream open for as long as the window is, and a window whose first paint
        // waited on a socket would come up empty.
        _session.Start();
    }

    // --- The chrome ----------------------------------------------------------------

    public TitleBarViewModel TitleBar { get; }

    public NavStripViewModel Nav { get; }

    public StatusBarViewModel StatusBar { get; }

    // --- The three destinations ----------------------------------------------------

    public SetupViewModel Setup { get; }

    public BroadcastViewModel Broadcast { get; }

    public ViewerViewModel Viewer { get; }

    // --- Outputs -------------------------------------------------------------------

    /// <summary>Where the window is. Written by <see cref="Show"/> and by nothing else.</summary>
    public Destination Current => _current;

    /// <summary>
    /// Whether broadcast can be reached. Written by <see cref="SetBroadcastAvailable"/>,
    /// and read by the strip to decide which segment dims.
    /// </summary>
    public bool IsBroadcastAvailable => _broadcastAvailable;

    /// <summary>
    /// What the body shows: the current destination's own view model. Typed as object
    /// because the three share nothing but the data template that draws them - forcing a
    /// common base on them would be a type invented for the shell's convenience.
    /// </summary>
    public object Body { get => _body; private set => Set(ref _body, value); }

    // --- Writes --------------------------------------------------------------------

    /// <summary>
    /// Moves the window to a destination. The one write of that state, and idempotent:
    /// showing the destination already showing re-renders and changes nothing.
    /// </summary>
    public void Show(Destination destination)
    {
        // Dispatches through the table, so a destination it does not name fails here rather
        // than three layers down in a body lookup.
        Assert.That(Destinations.LabelOf(destination).Length > 0, "a window shows a destination the table names", (int)destination);
        Assert.That(destination != Destination.Broadcast || _broadcastAvailable, "a window shows broadcast only while broadcast can be reached", (int)destination);

        Set(ref _current, destination, nameof(Current));
        Apply();
    }

    /// <summary>
    /// Says whether broadcast can be reached. Losing it while standing in it is a real case
    /// - a publish pipeline can die - so the window steps back to setup rather than leaving
    /// the strip with a segment that is dimmed and selected at once. Idempotent.
    /// </summary>
    public void SetBroadcastAvailable(bool available)
    {
        Set(ref _broadcastAvailable, available, nameof(IsBroadcastAvailable));

        if (!_broadcastAvailable && _current == Destination.Broadcast)
        {
            Set(ref _current, Destination.Setup, nameof(Current));
        }

        Apply();
    }

    /// <summary>
    /// Re-renders the window on any change to the running state, and settles whether broadcast
    /// can be reached from the one fact that decides it: whether anything is publishing. That
    /// is read from the session rather than told to the shell, so a pipeline that died while
    /// the reader was standing on the broadcast screen takes them back to setup by itself.
    /// </summary>
    private void OnSessionChanged()
    {
        SetBroadcastAvailable(_session.Publish?.Live is not null);
        OpenBroadcastIfAsked();
    }

    /// <summary>
    /// Takes the news that a start setup asked for was accepted, and records whether the reader
    /// wants to be moved. The move itself is attempted at once, because the live state can
    /// already have arrived on the event stream by the time the reply did.
    /// </summary>
    private void OnWentLive()
    {
        _opensBroadcast = Setup.OpensBroadcastWindow;
        OpenBroadcastIfAsked();
    }

    /// <summary>
    /// Moves to broadcast once the stream the reader started is in force. Idempotent, and it
    /// consumes the request: a later stream somebody else started is not a second reason to
    /// take this window off whatever it is showing.
    /// </summary>
    private void OpenBroadcastIfAsked()
    {
        if (!_opensBroadcast || !_broadcastAvailable)
        {
            return;
        }

        _opensBroadcast = false;
        Show(Destination.Broadcast);
    }

    /// <summary>
    /// The one render function. Safe to run twice: every child's own apply is idempotent,
    /// so an unchanged pass writes no property and fires no binding.
    /// </summary>
    public void Apply()
    {
        // Pushed, not read through: the bands are told the whole of their input in one write
        // each, so none of them can be half-updated between two facts that must agree.
        //
        // The stream's own name once there is one, because that is what the window is showing.
        // It is the backend's name for it and never one composed here.
        TitleBar.Show(_current, _session.Publish?.Live?.Publish?.Name is { Length: > 0 } name ? name : Idle);

        // All three, not only the current one. A destination that rendered only while it was
        // on screen would come back stale, and the body swap would show the last state it
        // had rather than the one the model is in now.
        Setup.Apply();
        Broadcast.Apply();
        Viewer.Apply();

        // After the bodies, so the strip's pill and the band's figures are the ones the
        // destinations have just derived rather than the ones they held before this pass.
        //
        // The strip's timer is the broadcast screen's own reading of the encoder clock, read
        // back rather than composed again here: the pill in the chrome and the pill in the
        // header say the same thing, and two derivations of one figure is how they would
        // eventually stop doing so.
        Nav.Show(_current, _broadcastAvailable, Broadcast.Snapshot.Elapsed);
        RenderStatusBar();

        Body = BodyFor(_current);

        Assert.That(_current != Destination.Broadcast || _broadcastAvailable, "a window shows broadcast only while broadcast can be reached", _broadcastAvailable);
        Assert.That(Nav.SelectedTab?.Value == _current, "the strip and the body stand in one destination", (int)_current);
    }

    /// <summary>
    /// What the shell does with a request the live screen raised. Only the two that
    /// navigate are the shell's; the rest belong to the publisher, and answering them here
    /// would be the shell inventing behaviour it does not own.
    /// </summary>
    private void OnBroadcastRequested(BroadcastAction action)
    {
        if (action == BroadcastAction.EditInSetup)
        {
            Show(Destination.Setup);
        }
    }

    /// <summary>
    /// Hands the band the destination it stands in and the figures that destination knows.
    /// Read through on every call and never cached here, so the band and the tiles cannot
    /// disagree about how many streams are on screen. Idempotent.
    /// </summary>
    private void RenderStatusBar()
        => StatusBar.Show(_current, Viewer.ShownSummary, Viewer.Figures, Viewer.Hint);

    /// <summary>
    /// Which view model the body shows. Exhaustive, so a destination added without a body
    /// fails here instead of rendering an empty pane.
    /// </summary>
    private object BodyFor(Destination destination) => destination switch
    {
        Destination.Setup => Setup,
        Destination.Broadcast => Broadcast,
        Destination.Viewer => Viewer,
        _ => Assert.Never<object>("unexpected destination", (int)destination),
    };
}
