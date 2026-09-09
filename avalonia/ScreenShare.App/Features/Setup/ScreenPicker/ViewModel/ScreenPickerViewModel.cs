using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Setup.ScreenPicker.Model;
using ScreenShare.App.Features.Setup.ShareQuestion.ViewModel;
using ScreenShare.App.Features.Viewer.Tile.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.ScreenPicker.ViewModel;

/// <summary>
/// One live picture per screen, so a screen is picked by looking rather than by index.
/// Costs one screen capture per screen and nothing else: nothing is encoded, no bandwidth is spent,
/// and the relay is no party to it.
/// Which outputs exist is the catalog's answer and which entries are greyed is the resolved form's,
/// both read through on every pass (<c>docs/ipc-api.md</c>).
/// The pictures and their arrangement are what this adds.
///
/// The pictures themselves belong to the question, which converges every screen its choosers want at once
/// (<see cref="MonitorPreviews"/>).
/// What is named here is <see cref="Wanted"/>: up while the grid is on screen with the window in front,
/// empty as soon as either stops holding.
/// Where no screen can be read apart from another, or the capture backend takes no monitor index, nothing is drawn
/// and the plain control is what is left.
/// </summary>
public sealed class ScreenPickerViewModel : Observable
{
    private readonly Session _session;
    private readonly MonitorPreviews _previews;

    /// <summary>Writes the picked screen into the form session's draft.</summary>
    private readonly Action<int> _choose;

    /// <summary>
    /// One command per output, made once, so an unchanged pass produces rows that compare equal
    /// and the collection is left alone.
    /// </summary>
    private readonly Dictionary<int, DelegateCommand> _select = [];

    /// <summary>Control in the tree, in a window that is in front. Written by the view.</summary>
    private bool _showing;

    /// <summary>Question this grid answers is on screen. Written by the question drawing it.</summary>
    private bool _drawn;

    /// <summary><c>publish.monitor</c> as the form last resolved it. Null before a form arrives.</summary>
    private Field? _field;

    /// <param name="choose">
    /// Draft write, handed in because the draft is the form session's
    /// and a second writer would be a second copy of what the settings say.
    /// </param>
    public ScreenPickerViewModel(Session session, MonitorPreviews previews, Action<int> choose)
    {
        Assert.NotNull(session, "a screen picker draws this machine's outputs");
        Assert.NotNull(previews, "a screen picker draws the question's own previews");
        Assert.NotNull(choose, "a screen picker writes the screen it was told to pick");

        _session = session;
        _previews = previews;
        _choose = choose;
    }

    // --- Outputs ------------------------------------------------------------------

    private bool _isVisible;
    private string _notice = "";
    private bool _hasNotice;

    /// <summary>One row per output, in the order the catalog enumerated them. Empty while nothing is drawn.</summary>
    public ObservableCollection<ScreenChoice> Screens { get; } = [];

    /// <summary>Screens this grid is asking the backend to read. Empty while it draws nothing.</summary>
    public IReadOnlyList<int> Wanted { get; private set; } = [];

    /// <summary>
    /// Whether the grid is drawn at all.
    /// False on a session that cannot read one screen apart from another,
    /// on a capture backend that takes no monitor index, and on a machine whose outputs could not be enumerated.
    /// Each is explained in its own words by the plain control beneath.
    /// </summary>
    public bool IsVisible { get => _isVisible; private set => Set(ref _isVisible, value); }

    /// <summary>
    /// What the pictures are, said once above the grid rather than per tile.
    /// Empty while the grid is not drawn.
    /// </summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    // --- Inputs -------------------------------------------------------------------

    /// <summary>
    /// Screen setting as the form resolved it, and whether the question it answers is on screen.
    /// Both are the question's to say.
    /// Neither is read off a widget.
    /// </summary>
    public void Apply(Field? monitor, bool drawn)
    {
        _field = monitor;
        _drawn = drawn;
        Render();
    }

    /// <summary>
    /// Named write of whether the grid is being looked at, idempotent: a value it already holds renders
    /// and converges to the same world.
    /// Called by the view, tree membership and window activation being visible to the control and the platform alone.
    /// </summary>
    public void SetShowing(bool showing)
    {
        _showing = showing;
        Render();
    }

    // --- Lifecycle ------------------------------------------------------------------

    /// <summary>
    /// The one render function.
    /// Every output is written on every pass, so neither a row nor a wanted screen can stick.
    /// </summary>
    public void Render()
    {
        IsVisible = Offered();

        var monitors = IsVisible ? _session.Monitors : [];
        Wanted = [.. monitors.Select(monitor => monitor.Index)];

        var rows = new List<ScreenChoice>(monitors.Count);
        foreach (var monitor in monitors)
        {
            // Made from what the backend reports reading, and dropped with it.
            var tile = _previews.TileOf(monitor.Index);
            // No sample: a screen is read rather than received, so no decode holds counters about it
            // (Features/Viewer/Tile/Model/TileStats.cs).
            tile?.Apply(TilePipeline.Of(_previews.Previewed(monitor.Index)), sample: null);

            var option = OptionOf(monitor.Index);
            rows.Add(new ScreenChoice(
                monitor.Index,
                _session.Words.Name("publish.monitor", monitor.Index.ToString()),
                IsSelected: Selected() == monitor.Index,
                // An output the form does not offer is one the settings cannot reach,
                // a monitor unplugged since the value was stored being the case, so the row is drawn
                // and cannot be picked.
                // Same treatment the dropdown beneath gives that entry.
                IsEnabled: option?.Enabled ?? false,
                Reason: Statements.Of(option?.Reason),
                tile,
                Placeholder: tile is null ? _previews.PlaceholderFor(monitor.Index) : "",
                SelectCommandOf(monitor.Index)));
        }

        Reconcile.Onto(Screens, rows);

        Notice = NoticeFor();
        HasNotice = Notice.Length > 0;

        Assert.That(HasNotice == (Notice.Length > 0), "a notice and its sentence agree", HasNotice);
        Assert.That(IsVisible || Screens.Count == 0, "a hidden picker offers no screen", Screens.Count);
        Assert.That(IsVisible || Wanted.Count == 0, "a hidden picker reads no screen", Wanted.Count);
    }

    /// <summary>
    /// What stands above the control, in the order the states happen in, the two being different news.
    /// A drawing grid says what its pictures are, and before that says nothing is being shared yet.
    /// A machine that could have shown pictures and cannot says why in the backend's own statement,
    /// an absence with no reason beside it reading as a fault rather than as how the session works.
    /// The second does not wait on <see cref="_showing"/>: a sentence costs nothing to draw,
    /// a picture costs a screen capture.
    /// </summary>
    private string NoticeFor()
    {
        if (IsVisible)
        {
            return Cards.ScreenPickerCost;
        }

        if (_drawn && _session.NoMonitorPreview is not null && Editable())
        {
            return Statements.Of(_session.NoMonitorPreview);
        }

        return "";
    }

    /// <summary>
    /// Whether a screen is the reader's to pick: the form is offering the setting, left it editable,
    /// and something was enumerated to pick between.
    /// All three hold before the absence of pictures is worth a word,
    /// a capture backend that chooses its own source already explaining itself on the disabled control.
    ///
    /// The visibility read is what ties the grid to the kind:
    /// the backend hides this control under a window and a rectangle
    /// (<c>backend/internal/form/share.go</c>).
    /// </summary>
    private bool Editable()
        => (_field?.Visible ?? false) && (_field?.Enabled ?? false) && _session.Monitors.Count > 0;

    /// <summary>
    /// Whether the grid has anything to draw: the question on screen with the window in front,
    /// one screen readable apart from another, and <see cref="Editable"/>.
    /// Every one is read through rather than remembered,
    /// so a capture backend changed elsewhere takes the grid away on the next pass with nothing here to clear.
    /// </summary>
    private bool Offered() => _drawn && _showing && _session.NoMonitorPreview is null && Editable();

    /// <summary>Screen the draft names. -1 before a form arrives.</summary>
    private int Selected()
        => _field?.Value?.KindCase == FieldValue.KindOneofCase.Number ? (int)_field.Value.Number : -1;

    /// <summary>Form's entry for one screen. Null for an output it does not offer.</summary>
    private FieldOption? OptionOf(int monitor)
    {
        if (_field is null)
        {
            return null;
        }

        var value = monitor.ToString();
        foreach (var option in _field.Options)
        {
            if (option.Value == value)
            {
                return option;
            }
        }

        return null;
    }

    private DelegateCommand SelectCommandOf(int monitor)
    {
        if (_select.TryGetValue(monitor, out var held))
        {
            return held;
        }

        var command = new DelegateCommand(() => _choose(monitor));
        _select[monitor] = command;
        return command;
    }
}
