using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.TitleBar.ViewModel;

/// <summary>
/// The title bar's state: the one line that says which window this is, and the three
/// commands the window controls fire.
///
/// Minimise, maximise and close are the only things on any screen that a view model cannot
/// carry out by itself - they are the platform's, not the app's. They arrive through
/// <see cref="Attach"/> rather than being reached for, so the bar stays constructible in a
/// test and the window keeps being built around the view model instead of the other way
/// round.
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
    /// The window's identity, written in one go so the destination and the session it
    /// belongs to cannot disagree in the title. Idempotent.
    /// </summary>
    public void Show(Destination current, string session)
    {
        Assert.NotNull(session, "a window title names the session it belongs to");

        _current = current;
        _session = session;
        Apply();
    }

    /// <summary>
    /// Whether the window is maximised. The middle caption button is one button with two
    /// meanings on Windows - maximise while the window is normal, restore while it is not -
    /// so the bar is told the state rather than inferring it from the command it last fired,
    /// which would be a second, drifting copy of what the window actually is. Idempotent.
    /// </summary>
    public void ShowMaximised(bool maximised)
    {
        _maximised = maximised;
        Apply();
    }

    /// <summary>
    /// Hands the bar the three things only a window can do. Idempotent: attaching twice
    /// leaves one set of actions, and the commands stay enabled either way, because a
    /// window control that greys itself out before composition finishes reads as a fault.
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

    /// <summary>The window title, as the title bar draws it and as the OS reads it.</summary>
    public string Title { get => _title; private set => Set(ref _title, value); }

    /// <summary>Which of the middle button's two glyphs is drawn: restore, or maximise.</summary>
    public bool IsMaximised { get => _isMaximised; private set => Set(ref _isMaximised, value); }

    /// <summary>What that button is called in the state it is in.</summary>
    public string MaximiseLabel { get => _maximiseLabel; private set => Set(ref _maximiseLabel, value); }

    /// <summary>
    /// The one render function. The design shows a title only for the viewer window
    /// ("Viewer — lab-04"), so the pattern - destination, em dash with a space either side,
    /// session - is this module's reading of it, applied to all three the same way.
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
