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
/// Window's own state, and sole owner of the one fact the rest of the screen turns on: which destination
/// is showing.
/// A band holds no destination of its own and is told one on every render pass, so a lit segment cannot disagree
/// with the body.
///
/// Every destination is reachable at all times, broadcast included: what it reports about the stream that has
/// just ended is what a publisher goes looking for once it has (<c>docs/design-language.md</c>, "Surfaces and shape").
///
/// <see cref="Show"/> is the named write.
/// <see cref="Apply"/> is the one render function, pushing the state into every child and picking the body
/// on every pass, off branches included (<c>docs/development-principles.md</c>, "One render function per component").
/// </summary>
public sealed class ShellViewModel : Observable
{
    /// <summary>What the title says in place of a stream name while nothing is publishing.</summary>
    private const string Idle = "no stream";

    /// <summary>
    /// Destination the window opens on.
    /// The viewer is the one screen the design states end to end, so the first paint is drawn from stated copy
    /// rather than from a state this module chose.
    /// </summary>
    private Destination _current = Destination.Viewer;

    /// <summary>Running state, owned once for the window and read through by everything drawing from it.</summary>
    private readonly Session _session;

    /// <summary>
    /// Settings draft and the form it resolves to, owned once for the window and read through by the destinations
    /// that edit settings.
    /// </summary>
    private readonly FormSession _form;

    private object _body;

    public ShellViewModel()
    {
        TitleBar = new TitleBarViewModel();
        Nav = new NavStripViewModel(Show);
        StatusBar = new StatusBarViewModel();

        // Go control plane over the local socket, constructed here because the shell owns the connection.
        // Every value, label, greying and figure a destination draws arrives through it already decided
        // (docs/ipc-api.md).
        //
        // Constructing opens nothing: the connection is made by the first read and remade by the next one after
        // a failure, so a window whose backend is not up paints its notice and reaches a backend once there is one.
        // An absent backend is an Umgebungsfehler the window survives.
        //
        // Reads are asynchronous because a socket is, so an answer lands on whichever thread the transport
        // completed on and is marshalled back before any bound property is written.
        // Posted rather than checked first, a post from the UI thread already being right.
        var backend = new ControlBackend();
        var dispatch = (Action<Action>)(action => Dispatcher.UIThread.Post(action));

        // One session for the window, and the one owner of the running state.
        // Broadcast and the viewer describe that session from two angles, so a session each would be two
        // windows' worth of reads and two answers to what is publishing.
        _session = new Session(backend, dispatch);

        // One draft for the window, and the one owner of the settings nobody has committed yet.
        // Setup configures what this machine sends and the viewer how it receives, and a draft each would be two
        // copies of one message, with the publish commit persisting whichever of them it held
        // (Backend/FormSession.cs).
        _form = new FormSession(backend, _session, dispatch);

        Setup = new SetupViewModel(backend, _form, _session, dispatch);
        Broadcast = new BroadcastViewModel(backend, _form, _session, dispatch);
        Viewer = new ViewerViewModel(backend, _form, _session, dispatch);

        // A decode is keyed by stream and leg, and this machine's own stream is one the grid may tile, so
        // the preview's end-to-end route and a grid tile can be the same decode.
        // A stop from either would take the picture out of the other, so the card reads the grid's answer
        // through before closing anything (Preview/ViewModel/PreviewViewModel.cs).
        // Wired here because the shell holds both screens, and the viewer does not exist yet when
        // the broadcast screen is built.
        Broadcast.Preview.SetGridLeg(stream => Viewer.TileOf(stream)?.Transport ?? "");

        // Every destination re-renders on any change, the chrome reading them all: the strip's pill says whether
        // this machine is sharing, and the band prints the viewer's figures from any destination.
        _session.Changed += Apply;

        // Levels have their own notification and reach the viewer alone.
        // A level moves fifteen times a second, and the change notification re-renders every destination
        // (Backend/Session.cs).
        _session.Metered += Viewer.Meter;

        // The pointer rides that notification and reaches the preview alone.
        // The position leaves the capture rather than being drawn into it, so the preview is the only place
        // it is drawn at all.
        _session.Metered += () => Broadcast.Preview.Point(_session.Pointer);

        _body = BodyFor(_current);

        // A reader toggling a chip moves the viewer's figures without going through a write of the shell's, so
        // the band re-renders off the viewer's own notification rather than waiting for a destination change.
        // A stream taken fullscreen from a tile's menu arrives the same way, and takes the bands off the window.
        Viewer.PropertyChanged += (_, _) =>
        {
            RenderStatusBar();
            RenderChrome();
        };

        // The live screen names the navigation and the shell performs it, so a value stays editable in one window
        // (docs/design-language.md, "Ownership").
        // Every other action that screen raises it performs itself against the backend.
        Broadcast.ActionRequested += OnBroadcastRequested;

        // Setup owns the commit and performs it.
        // Where the window goes afterwards is the window's.
        // Moved on the accepted reply rather than on the live state landing: the destination is reachable either
        // way, so the screen the start leads to comes up at once and fills in as the event stream reports what
        // the stream became.
        Setup.WentLive += () => Show(Destination.Broadcast);

        // Rendered before anything is read, so the window paints a complete view model whether or not
        // the backend is reachable, and the first state lands on a later pass.
        Apply();

        // Not awaited: it reads state and then holds the event stream open for as long as the window lives,
        // and a first paint waiting on a socket would come up empty.
        _session.Start();
    }

    // --- The chrome ----------------------------------------------------------------

    public TitleBarViewModel TitleBar { get; }

    public NavStripViewModel Nav { get; }

    public StatusBarViewModel StatusBar { get; }

    // --- The destinations ----------------------------------------------------------

    public SetupViewModel Setup { get; }

    public BroadcastViewModel Broadcast { get; }

    public ViewerViewModel Viewer { get; }

    // --- Outputs -------------------------------------------------------------------

    /// <summary>
    /// Showing destination's own view model.
    /// Typed as object: the destinations share nothing but the data template that draws them, and a common base
    /// would be a type invented for the shell's convenience.
    /// </summary>
    public object Body { get => _body; private set => Set(ref _body, value); }

    private bool _hasChrome = true;
    private bool _hasCaption;

    /// <summary>
    /// Whether the window draws its own bands.
    /// A stream filling the window takes every band off it: a picture under chrome is the app filling the screen,
    /// where the reader asked for the stream to fill it, which is what separates this from a maximised window
    /// (<c>Features/Viewer/View/ViewerView.axaml</c>).
    /// </summary>
    public bool HasChrome { get => _hasChrome; private set => Set(ref _hasChrome, value); }

    /// <summary>
    /// Whether the app's own title band is drawn.
    /// Two facts read in one place so the band has one writer: whether this app draws the caption at all
    /// (<c>Features/Shell/Model/WindowChrome.cs</c>), and whether the window draws chrome.
    /// </summary>
    public bool HasCaption { get => _hasCaption; private set => Set(ref _hasCaption, value); }

    // --- Writes --------------------------------------------------------------------

    /// <summary>
    /// Moves the window to a destination.
    /// The one write of that state, and idempotent: the destination already showing re-renders and changes
    /// nothing.
    /// </summary>
    public void Show(Destination destination)
    {
        // Read through the table, so a destination it does not name fails here rather than in a body lookup three
        // layers down.
        Assert.That(Destinations.LabelOf(destination).Length > 0, "a window shows a destination the table names", (int)destination);

        // The field and not a notification: where the window stands reaches the screen through the segments and
        // the body Apply writes, and nothing binds the destination itself.
        _current = destination;
        Apply();
    }

    /// <summary>
    /// One render function.
    /// Safe to run twice: every child's apply is idempotent, so an unchanged pass writes no property and fires no
    /// binding.
    /// </summary>
    public void Apply()
    {
        // Pushed rather than read through: a band takes its whole input in one write, so it cannot be half updated
        // between two facts that have to agree.
        //
        // The stream name is the backend's, never one composed here.
        TitleBar.Show(_current, _session.Publish?.Live?.Publish?.Name is { Length: > 0 } name ? name : Idle);

        // Every destination, not the showing one alone.
        // A destination rendered only while on screen comes back stale, and the body swap draws the state it last
        // held rather than the one the model is in.
        Setup.Apply();
        Broadcast.Apply();
        Viewer.Apply();

        // After the bodies, so the strip's pill and the band's figures are what the destinations derived
        // on this pass rather than what they held before it.
        //
        // Both facts are read back off the broadcast screen's reading instead of being composed again, so
        // the pill in the chrome and the pill in the header cannot disagree.
        Nav.Show(_current, Broadcast.Snapshot.IsLive, Broadcast.Snapshot.Elapsed);
        RenderStatusBar();
        RenderChrome();

        Body = BodyFor(_current);

        Assert.That(Nav.SelectedTab?.Value == _current, "the strip and the body stand in one destination", (int)_current);
        Assert.That(!HasCaption || HasChrome, "the title band is one of the bands the window either draws or does not", HasCaption, HasChrome);
    }

    /// <summary>
    /// What the shell does with a request the live screen raised.
    /// A request that navigates is the shell's.
    /// The rest are the publisher's, and answering one here would be the shell inventing behaviour it does not
    /// own.
    /// </summary>
    private void OnBroadcastRequested(BroadcastAction action)
    {
        if (action == BroadcastAction.EditInSetup)
        {
            Show(Destination.Setup);
        }
    }

    /// <summary>
    /// Hands the band the destination it stands in and the figures that destination derived.
    /// Read through on every call and cached nowhere here, so the band and the tiles cannot disagree about what
    /// is on screen.
    /// Idempotent.
    /// </summary>
    private void RenderStatusBar()
        => StatusBar.Show(_current, Viewer.ShownSummary, Viewer.Figures, Viewer.Hint);

    /// <summary>
    /// Whether the window's bands are drawn, derived from the destination and what that destination shows.
    /// Read through rather than written by whoever moved it, so a stream taken fullscreen from a tile's menu takes
    /// the bands with it without telling the shell.
    /// Idempotent.
    /// </summary>
    private void RenderChrome()
    {
        HasChrome = !(_current == Destination.Viewer && Viewer.HasFullscreen);
        HasCaption = WindowChrome.AppDrawsCaption && HasChrome;
    }

    /// <summary>
    /// Which view model the body shows.
    /// Exhaustive, so a destination added without a body fails here rather than rendering an empty pane.
    /// </summary>
    private object BodyFor(Destination destination) => destination switch
    {
        Destination.Setup => Setup,
        Destination.Broadcast => Broadcast,
        Destination.Viewer => Viewer,
        _ => Assert.Never<object>("unexpected destination", (int)destination),
    };
}
