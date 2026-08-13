using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.ConfigCard.ViewModel;

/// <summary>
/// The active configuration, read-only while live.
///
/// It carries no setters on purpose: every setting here reaches a running pipeline by restarting it, and a
/// control that appears both here and in setup is a control with two owners.
/// The one thing this card can do is leave - <see cref="EditInSetupCommand"/> raises
/// <see cref="EditRequested"/> and lets the shell navigate.
///
/// The note at the foot of it used to divide the settings into ones needing a restart and ones that did not.
/// There was never a live-safe apply for the second group to use, so the division described nothing;
/// <see cref="Copy.Cards.ConfigReadOnly"/> states the one rule that holds.
/// </summary>
public sealed class ConfigCardViewModel : Observable
{
    /// <summary>Raised when the reader asks to go and edit. The card never edits anything itself.</summary>
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
    /// What the pipeline reports it is running: one row per group of the form resolved against the settings
    /// the running pipeline was built from.
    /// An input rather than an output, since nobody writes it from this screen.
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

    // --- Outputs ------------------------------------------------------------------

    private string _notice = "";
    private bool _hasRows;

    public ObservableCollection<ConfigRow> Rows { get; }

    public DelegateCommand EditInSetupCommand { get; }

    /// <summary>
    /// What stands in for the rows, and there is only one state it can stand in for.
    ///
    /// It used to say nothing was publishing.
    /// That reads as the obvious reason for an empty card and it is one this card can never be showing: the
    /// broadcast destination exists only while a stream is live, and stopping one navigates the window off it
    /// (<c>Features/Shell/ViewModel/ShellViewModel.cs</c>, <c>SetBroadcastAvailable</c>).
    /// So an empty row set here means the rows have not arrived yet - the screen resolves a form for the
    /// settings the running pipeline was built from, and the first passes of every broadcast happen before
    /// that answer lands.
    /// </summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    /// <summary>
    /// Why the card takes no edits, read from the copy table rather than spelled into the markup - which is
    /// where the sentence that divided the settings into restarting and live-safe ones sat, unread beside the
    /// card that says nothing here is live-safe.
    /// </summary>
    public string ReadOnly => Copy.Cards.ConfigReadOnly;

    public bool HasRows { get => _hasRows; private set => Set(ref _hasRows, value); }

    /// <summary>
    /// The one render function.
    /// Rows are records, so a second pass over the same reported configuration leaves the bound list
    /// untouched.
    /// </summary>
    public void Apply()
    {
        Reconcile.Onto(Rows, Reported);
        EditInSetupCommand.Refresh();

        HasRows = Rows.Count > 0;
        Notice = HasRows ? "" : Copy.Cards.ConfigUndescribed;

        Assert.That(Rows.Count == Reported.Count, "a row per reported setting", Rows.Count, Reported.Count);
        Assert.That(HasRows == (Notice.Length == 0), "rows and the sentence standing in for them are never both on screen", HasRows);
    }
}
