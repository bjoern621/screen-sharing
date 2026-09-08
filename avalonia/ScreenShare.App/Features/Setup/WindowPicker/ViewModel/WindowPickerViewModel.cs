using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.WindowPicker.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.WindowPicker.ViewModel;

/// <summary>
/// One row per window this machine has open, so a window is picked by what it is called
/// rather than by a handle in a dropdown.
///
/// Which handles can be picked is the resolved form's answer, read through on every pass
/// (<c>docs/ipc-api.md</c>).
/// What this adds is the read that names them: the form carries handles, and the titles behind them
/// cross on a call of their own (<c>ListShareWindows</c>).
///
/// <b>The list is read when the question comes up, and again on request.</b>
/// Windows open and close while nobody is looking, so one held across a visit would offer one that is gone.
/// A pass per keystroke asks for nothing, the read following the question being put.
/// </summary>
public sealed class WindowPickerViewModel : Observable
{
    private readonly IBackend _backend;
    private readonly Action<Action> _dispatch;

    /// <summary>Writes the picked window into the form session's draft.</summary>
    private readonly Action<string> _choose;

    /// <summary>
    /// One command per handle, made once, so an unchanged pass produces rows that compare equal.
    /// </summary>
    private readonly Dictionary<string, DelegateCommand> _select = [];

    /// <summary>Question is being put, which is what a read follows. Written by the step.</summary>
    private bool _drawn;

    /// <summary>Whether the list was read for this visit. Cleared when the question goes away.</summary>
    private bool _read;

    /// <summary><c>publish.share_window</c> as the form last resolved it. Null before a form arrives.</summary>
    private Field? _field;

    public WindowPickerViewModel(IBackend backend, Action<Action> dispatch, Action<string> choose)
    {
        Assert.NotNull(backend, "a window picker reads the windows this machine has open");
        Assert.NotNull(dispatch, "a window picker marshals a read back to the UI loop");
        Assert.NotNull(choose, "a window picker writes the window it was told to pick");

        _backend = backend;
        _dispatch = dispatch;
        _choose = choose;

        RefreshCommand = new PendingCommand(ReadAsync, dispatch, () => _drawn);
    }

    // --- Outputs ------------------------------------------------------------------

    private bool _isVisible;
    private bool _isEmpty;

    /// <summary>One row per window the form offers, in its order. Empty while nothing is drawn.</summary>
    public ObservableCollection<WindowChoice> Windows { get; } = [];

    /// <summary>Windows as the last read found them, which is what names the handles the form offers.</summary>
    public IReadOnlyList<ShareWindow> Listed { get; private set; } = [];

    /// <summary>
    /// Whether the list is drawn at all, which is whether the form is offering the control.
    /// False under the other two kinds and on a capture backend that reads no window.
    /// </summary>
    public bool IsVisible { get => _isVisible; private set => Set(ref _isVisible, value); }

    /// <summary>Whether the list is drawn and has nothing in it.</summary>
    public bool IsEmpty { get => _isEmpty; private set => Set(ref _isEmpty, value); }

    /// <summary>Reads the list again, for a window opened since the question came up.</summary>
    public PendingCommand RefreshCommand { get; }

    // --- Inputs -------------------------------------------------------------------

    /// <summary>
    /// Window setting as the form resolved it, the words naming its entries,
    /// and whether the question is being put. All three are the step's to say.
    /// </summary>
    public void Apply(Field? window, Vocabulary words, bool drawn)
    {
        Assert.NotNull(words, "rendering the window list needs the vocabulary naming its entries");

        _field = window;
        _drawn = drawn;
        Render(words);

        // After the render, so a first pass draws the rows the form already carries
        // rather than waiting on a call.
        if (_drawn && !_read)
        {
            _read = true;
            Read();
        }
        else if (!_drawn)
        {
            _read = false;
        }
    }

    // --- Lifecycle ------------------------------------------------------------------

    /// <summary>
    /// The one render function.
    /// Safe to run twice: rows are records over held commands, so an unchanged pass leaves the collection alone.
    /// </summary>
    private void Render(Vocabulary words)
    {
        IsVisible = (_field?.Visible ?? false) && (_field?.Enabled ?? false);

        var rows = new List<WindowChoice>(_field?.Options.Count ?? 0);
        if (IsVisible && _field is not null)
        {
            var picked = _field.Value?.KindCase == FieldValue.KindOneofCase.Text ? _field.Value.Text : "";
            foreach (var option in _field.Options)
            {
                rows.Add(new WindowChoice(
                    option.Value,
                    words.Name(ShareLayout.WindowKey, option.Value),
                    words.WindowSize(option.Value),
                    IsSelected: option.Value == picked,
                    option.Enabled,
                    Statements.Of(option.Reason),
                    SelectCommandOf(option.Value)));
            }
        }

        Reconcile.Onto(Windows, rows);

        IsEmpty = IsVisible && Windows.Count == 0;
        RefreshCommand.Refresh();

        Assert.That(IsVisible || Windows.Count == 0, "a hidden list offers no window", Windows.Count);
    }

    /// <summary>
    /// Reads the list without a caller waiting on it, for the transition that puts the question up.
    /// </summary>
    private async void Read()
    {
        try
        {
            await ReadAsync().ConfigureAwait(false);
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <summary>
    /// Reads the windows this machine has open.
    /// A read that fails leaves the rows named by their handles, which is a list that works and reads badly
    /// rather than none.
    /// </summary>
    private async Task ReadAsync()
    {
        IReadOnlyList<ShareWindow> listed;
        try
        {
            listed = await _backend.ShareWindowsAsync().ConfigureAwait(true);
        }
        catch (BackendUnavailableException)
        {
            listed = [];
        }

        _dispatch(() =>
        {
            Listed = listed;
            Changed?.Invoke();
        });
    }

    /// <summary>
    /// Raised when a read landed, so whoever holds the vocabulary renders again with the names in hand.
    /// </summary>
    public event Action? Changed;

    private DelegateCommand SelectCommandOf(string handle)
    {
        if (_select.TryGetValue(handle, out var held))
        {
            return held;
        }

        var command = new DelegateCommand(() => _choose(handle));
        _select[handle] = command;
        return command;
    }
}
