using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.ConfigCard.ViewModel;

/// <summary>
/// The active configuration, read-only while live.
///
/// No setters: every setting here reaches a running pipeline by restarting it, and a control appearing both
/// here and in setup is a control with two owners.
/// The one thing the card can do is leave, through <see cref="EditInSetupCommand"/> and
/// <see cref="EditRequested"/>, which the shell navigates on.
/// </summary>
public sealed class ConfigCardViewModel : Observable
{
    /// <summary>Raised when the reader asks to edit. The card writes nothing itself.</summary>
    public event Action? EditRequested;

    public ConfigCardViewModel()
    {
        Rows = [];
        EditInSetupCommand = new DelegateCommand(() => EditRequested?.Invoke());
        Apply();
    }

    // --- Inputs -------------------------------------------------------------------

    private IReadOnlyList<ConfigRow> _reported = [];

    /// <summary>
    /// What the pipeline reports it is running: a row per group of the form resolved against the settings the
    /// running pipeline was built from.
    /// An input, since nothing on this screen writes it.
    /// </summary>
    public IReadOnlyList<ConfigRow> Reported
    {
        get => _reported;
        set
        {
            Assert.NotNull(value, "a configuration card renders reported rows");

            if (Set(ref _reported, value))
            {
                Apply();
            }
        }
    }

    private bool _isLive;

    /// <summary>
    /// Whether a stream is in force.
    /// The card is drawn on a destination that is reachable with nothing publishing, so which absence
    /// <see cref="Notice"/> reports is not something an empty row set answers.
    /// </summary>
    public bool IsLive
    {
        get => _isLive;
        set
        {
            if (Set(ref _isLive, value))
            {
                Apply();
            }
        }
    }

    // --- Outputs ------------------------------------------------------------------

    private string _notice = "";
    private bool _hasRows;

    public ObservableCollection<ConfigRow> Rows { get; }

    public DelegateCommand EditInSetupCommand { get; }

    /// <summary>
    /// What stands in for the rows, and there are two states it can stand in for.
    ///
    /// While a stream runs, an empty row set means the rows have not arrived: the screen resolves a form for
    /// the settings the running pipeline was built from, and every broadcast's first passes happen before that
    /// answer lands.
    /// With nothing publishing there is no pipeline to describe at all, and a card saying it is reading one
    /// would wait on an answer nothing has been asked for.
    /// </summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    /// <summary>Why the card takes no edits, read from the copy table rather than spelled into the markup.</summary>
    public string ReadOnly => Copy.Cards.ConfigReadOnly;

    public bool HasRows { get => _hasRows; private set => Set(ref _hasRows, value); }

    /// <summary>
    /// The one render function.
    /// Rows are records, so a second pass over one reported configuration leaves the bound list untouched.
    /// </summary>
    public void Apply()
    {
        Reconcile.Onto(Rows, Reported);
        EditInSetupCommand.Refresh();

        HasRows = Rows.Count > 0;
        Notice = HasRows ? ""
            : IsLive ? Copy.Cards.ConfigUndescribed
            : Copy.Cards.ConfigIdle;

        Assert.That(Rows.Count == Reported.Count, "a row per reported setting", Rows.Count, Reported.Count);
        Assert.That(HasRows == (Notice.Length == 0), "rows and the sentence standing in for them are never both on screen", HasRows);
    }
}
