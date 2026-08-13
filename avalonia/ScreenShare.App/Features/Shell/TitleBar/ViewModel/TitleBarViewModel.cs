using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.TitleBar.ViewModel;

/// <summary>
/// The title bar's state: which window this is, and the commands its window controls fire.
///
/// Minimise, maximise and close are the platform's rather than the app's, and the only effects on any screen
/// a view model cannot carry out itself.
/// They arrive through <see cref="Attach"/> rather than being reached for, so the bar is constructible in a
/// test and the window is built around the view model rather than the other way round.
/// </summary>
public sealed class TitleBarViewModel : Observable
{
    private Action _minimise = static () => { };
    private Action _toggleMaximise = static () => { };
    private Action _close = static () => { };

    public TitleBarViewModel()
    {
        MinimiseCommand = new DelegateCommand(() => _minimise());
        MaximiseCommand = new DelegateCommand(() => _toggleMaximise());
        CloseCommand = new DelegateCommand(() => _close());
    }

    public DelegateCommand MinimiseCommand { get; }

    public DelegateCommand MaximiseCommand { get; }

    public DelegateCommand CloseCommand { get; }

    // --- What the shell says -------------------------------------------------------

    private Destination _current = Destination.Setup;
    private string _session = "";
    private bool _maximised;

    /// <summary>
    /// The window's identity, written in one go so the destination and the session cannot disagree in the
    /// title.
    /// Idempotent.
    /// </summary>
    public void Show(Destination current, string session)
    {
        Assert.NotNull(session, "a window title names the session it belongs to");

        _current = current;
        _session = session;
        Apply();
    }

    /// <summary>
    /// Whether the window is maximised.
    /// Told rather than inferred from the command last fired: the window state also moves from the desktop,
    /// from a double click on the band and from a snap gesture.
    /// Idempotent.
    /// </summary>
    public void ShowMaximised(bool maximised)
    {
        _maximised = maximised;
        Apply();
    }

    /// <summary>
    /// Hands the bar the effects only a window can perform.
    /// Idempotent: attaching twice leaves one set of actions.
    /// The commands stay enabled before an attach as well, because a window control greying itself out until
    /// composition finishes reads as a fault.
    /// </summary>
    public void Attach(Action minimise, Action toggleMaximise, Action close)
    {
        Assert.NotNull(minimise, "a title bar needs a way to minimise its window");
        Assert.NotNull(toggleMaximise, "a title bar needs a way to maximise its window");
        Assert.NotNull(close, "a title bar needs a way to close its window");

        _minimise = minimise;
        _toggleMaximise = toggleMaximise;
        _close = close;
        Apply();
    }

    // --- Outputs -------------------------------------------------------------------

    private string _title = "";
    private bool _isMaximised;
    private string _maximiseLabel = "";

    /// <summary>Drawn by the band, and read by the desktop off the window.</summary>
    public string Title { get => _title; private set => Set(ref _title, value); }

    /// <summary>Picks the middle button's glyph: restore while true, maximise while false.</summary>
    public bool IsMaximised { get => _isMaximised; private set => Set(ref _isMaximised, value); }

    /// <summary>That button's tooltip, named for the state it is in.</summary>
    public string MaximiseLabel { get => _maximiseLabel; private set => Set(ref _maximiseLabel, value); }

    /// <summary>
    /// The one render function.
    /// The design names a title for the viewer window alone, so the pattern (destination, spaced em dash,
    /// session) is this module's reading of that one, applied to every destination.
    /// </summary>
    public void Apply()
    {
        var label = Destinations.LabelOf(_current);
        Title = _session.Length == 0 ? label : $"{label} — {_session}";

        IsMaximised = _maximised;
        MaximiseLabel = _maximised ? "Restore" : "Maximise";

        MinimiseCommand.Refresh();
        MaximiseCommand.Refresh();
        CloseCommand.Refresh();

        Assert.That(Title.Length > 0, "a window title says at least which destination it is", Title);
        Assert.That(MaximiseLabel.Length > 0, "the middle caption button is named in both states", MaximiseLabel);
    }
}
