using System.Collections.ObjectModel;
using System.Globalization;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Fields.Model;
using ScreenShare.App.Mvvm;
using TablerIcons;

namespace ScreenShare.App.Features.Fields.ViewModel;

/// <summary>
/// One control of the resolved form, as the screen draws it.
///
/// The shell's whole understanding of a setting: a key, a control kind mapped to a widget, and the statements
/// the backend made about it.
/// No rule is evaluated and no value invented here; which entries exist, which are reachable and why not
/// arrive already decided (docs/ipc-api.md, "The rule").
///
/// Every word is this side's.
/// The heading, the paragraph behind it, what an entry is called and the sentence standing in for a greyed one
/// are looked up by <see cref="Key"/> and by the entry's value (ScreenShare.App.Copy).
/// The backend sends <c>hevc_nvenc</c>; this is where that becomes something to read.
///
/// Inputs are <see cref="Text"/>, <see cref="Number"/>, <see cref="Slide"/> and <see cref="Flag"/>, the four
/// shapes a widget writes back.
/// A setter reports the change to whoever owns the draft and nothing else, since the next resolved form is the
/// answer to whether the change was legal.
/// <see cref="RefusedShown"/> is the one input reporting nothing: which entries a reader looks at changes no
/// setting, so it re-renders here and goes no further.
///
/// Outputs are written by <see cref="Apply"/> on every pass, including the branches that turn a control off.
/// The value departs from that split and is both: the backend may repair a draft and a shell adopts the
/// repaired one wholesale, so <see cref="Apply"/> assigns the inputs too.
/// The echo guard is what keeps an adopted value from reading back as a fresh edit.
/// </summary>
public sealed class FieldViewModel : Observable
{
    private readonly Action<string, FieldValue> _write;
    private readonly Dictionary<string, DelegateCommand> _choose = [];

    /// <summary>
    /// The shape of this field's value, as the last resolved form carried it.
    /// An option value crosses as a string whatever the field is, so this is what turns a pick back into the
    /// type the settings field holds; without it a select over a number would mark an entry and write zero.
    /// </summary>
    private FieldValue.KindOneofCase _kind = FieldValue.KindOneofCase.Text;

    /// <summary>
    /// True while <see cref="Apply"/> is assigning the inputs.
    /// A widget's setter checks it and reports nothing, so adopting a repaired value does not read as typing.
    /// </summary>
    private bool _adopting;

    /// <summary>
    /// How this field's entries are named, as the last pass was given it.
    /// Held rather than passed down because the entries are rebuilt inside the same pass that sets it, and a
    /// naming one pass behind would label a fresh list from a stale catalog.
    /// </summary>
    private Vocabulary _words = Vocabulary.Empty;

    /// <summary>
    /// Last field a pass was given, so revealing the refused entries renders from what the backend said rather
    /// than from what the widgets hold.
    /// </summary>
    private Field? _field;

    public FieldViewModel(string key, Action<string, FieldValue> write)
    {
        Assert.That(key.Length > 0, "a field is identified by the settings field it edits");
        Assert.NotNull(write, "a field needs somewhere to report what the user moved");

        Key = key;
        _write = write;
        Options = [];
        Shown = [];
        MenuRows = [];
        RevealCommand = new DelegateCommand(ToggleRefused);
    }

    /// <summary>The settings field this control edits, as <c>publish.codec</c>. Carried, never parsed.</summary>
    public string Key { get; }

    // --- Inputs -------------------------------------------------------------------

    private string _text = "";
    private decimal? _number;
    private double _slide;
    private bool _flag;

    /// <summary>A free-text value, in the type a text box binds.</summary>
    public string Text
    {
        get => _text;
        set
        {
            if (Set(ref _text, value) && !_adopting)
            {
                _write(Key, new FieldValue { Text = value });
            }
        }
    }

    /// <summary>A typed number, in the type a spinner binds. Null while the box is empty, and an empty box reports nothing.</summary>
    public decimal? Number
    {
        get => _number;
        set
        {
            if (Set(ref _number, value) && !_adopting && value is { } number)
            {
                _write(Key, new FieldValue { Number = (long)number });
            }
        }
    }

    /// <summary>The same settings value as <see cref="Number"/>, in the type a slider binds.</summary>
    public double Slide
    {
        get => _slide;
        set
        {
            if (Set(ref _slide, value) && !_adopting)
            {
                _write(Key, new FieldValue { Number = (long)Math.Round(value) });
            }
        }
    }

    public bool Flag
    {
        get => _flag;
        set
        {
            if (Set(ref _flag, value) && !_adopting)
            {
                _write(Key, new FieldValue { Flag = value });
            }
        }
    }

    private bool _refusedShown;

    /// <summary>
    /// Held here and not in the view: a list the reader opened is state the reader set, and the same answer has
    /// to reach a card list and a dropdown's rows alike.
    /// </summary>
    public bool RefusedShown
    {
        get => _refusedShown;
        set
        {
            if (Set(ref _refusedShown, value) && _field is not null)
            {
                Apply(_field, _words, Action);
            }
        }
    }

    // --- Outputs ------------------------------------------------------------------

    private string _label = "";
    private string _help = "";
    private string _doc = "";
    private string _unit = "";
    private string _reason = "";
    private string _note = "";
    private string _readback = "";
    private string _pickedLabel = "";
    private string _pickedNote = "";
    private bool _hasPickedNote;
    private bool _isVisible;
    private bool _isEnabled;
    private bool _appliesLive;
    private bool _hasHelp;
    private bool _hasDoc;
    private bool _hasReason;
    private bool _hasNote;
    private bool _hasUnit;
    private bool _isText;
    private bool _isNumber;
    private bool _isNumberSelect;
    private bool _isSlider;
    private bool _isToggle;
    private bool _isSelect;
    private bool _isRadio;
    private bool _isChoice;
    private bool _isReadonly;
    private double _minimum;
    private double _maximum;
    private double _step = 1;
    private decimal _numberMinimum;
    private decimal _numberMaximum;
    private decimal _numberStep = 1;
    private FieldAction? _action;
    private bool _hasAction;
    private string _actionNotice = "";
    private bool _hasActionNotice;
    private bool _hasRefused;
    private string _refusedCount = "";
    private Icons _refusedGlyph = Icons.IconChevronRight;

    /// <summary>
    /// The entries of a select, a radio or a number carrying a ladder. Empty on every other kind.
    /// Every one of them, refused included, in the order the form gave them; what a surface draws is
    /// <see cref="Shown"/> or <see cref="MenuRows"/>.
    /// </summary>
    public ObservableCollection<OptionViewModel> Options { get; }

    /// <summary>
    /// What a list of cards draws: the allowed entries, and the refused ones once asked for.
    /// The disclosure sits under the list, so what it reveals arrives above the control that revealed it.
    /// </summary>
    public ObservableCollection<OptionViewModel> Shown { get; }

    /// <summary>
    /// What an opened dropdown lists: <see cref="Shown"/> with the disclosure last.
    /// One collection because a flyout takes one item source, with nowhere beside the rows for a control to sit.
    /// </summary>
    public ObservableCollection<OptionViewModel> MenuRows { get; }

    /// <summary>Lists the refused entries, and hides them again.</summary>
    public DelegateCommand RevealCommand { get; }

    /// <summary>
    /// The effect offered beside this control, null where the screen offers none.
    /// The screen's own placement rather than anything the form described, and it writes this field through the
    /// path a keystroke takes (<see cref="FieldAction"/>).
    /// </summary>
    public FieldAction? Action { get => _action; private set => Set(ref _action, value); }

    public bool HasAction { get => _hasAction; private set => Set(ref _hasAction, value); }

    /// <summary>
    /// Why the effect beside this control is refused, or what its last attempt answered.
    /// Empty where there is no effect, or nothing to say about one.
    /// Lifted off the action rather than bound through it: a control offering none has no action to bind
    /// through, and a binding down a null path draws the absent sentence as an empty line instead of no line.
    /// </summary>
    public string ActionNotice { get => _actionNotice; private set => Set(ref _actionNotice, value); }

    public bool HasActionNotice { get => _hasActionNotice; private set => Set(ref _hasActionNotice, value); }

    public string Label { get => _label; private set => Set(ref _label, value); }

    /// <summary>The paragraph behind the control. The form is pedagogical, and this is where it teaches.</summary>
    public string Help { get => _help; private set => Set(ref _help, value); }

    /// <summary>The reference article for the concept, empty where the control has none.</summary>
    public string Doc { get => _doc; private set => Set(ref _doc, value); }

    public string Unit { get => _unit; private set => Set(ref _unit, value); }

    /// <summary>Why the control is inert, drawn in its place. Empty while it is live.</summary>
    public string Reason { get => _reason; private set => Set(ref _reason, value); }

    /// <summary>What the field means here that its label does not say. The control stays editable, so a note takes nothing away.</summary>
    public string Note { get => _note; private set => Set(ref _note, value); }

    /// <summary>The value as a read-back prints it, for a field with no input on it.</summary>
    public string Readback { get => _readback; private set => Set(ref _readback, value); }

    /// <summary>
    /// The picked entry's label, drawn on a closed dropdown.
    /// Falls back to the raw value for the case the contract allows: a legal value that is none of the entries
    /// offered.
    /// </summary>
    public string PickedLabel { get => _pickedLabel; private set => Set(ref _pickedLabel, value); }

    /// <summary>The picked entry's trailing note, which is what makes a closed dropdown honest.</summary>
    public string PickedNote { get => _pickedNote; private set => Set(ref _pickedNote, value); }

    public bool HasPickedNote { get => _hasPickedNote; private set => Set(ref _hasPickedNote, value); }

    /// <summary>False for a knob with no meaning outside one selection. The control is not drawn at all.</summary>
    public bool IsVisible { get => _isVisible; private set => Set(ref _isVisible, value); }

    /// <summary>
    /// True where the value is written to the pipeline already carrying the stream, so an edit costs nobody
    /// watching a reconnect.
    /// False where applying it replaces the encoder child and every viewer reconnects across the gap.
    ///
    /// The backend's answer per combination rather than a list held here: the engine behind the capture backend
    /// decides whether anything is live at all, and the codec and rate-control mode decide whether the encoder
    /// is being sent that value.
    /// A list on this side would go on promising a reconnect-free edit after the backend stopped delivering one
    /// (docs/field-availability.md, "A live stream blocks no field").
    /// </summary>
    public bool AppliesLive { get => _appliesLive; private set => Set(ref _appliesLive, value); }

    /// <summary>What changing an <see cref="AppliesLive"/> control costs, in the width a chip beside a label has.</summary>
    public string LiveNotice => Copy.Fields.LiveNotice;

    public string RefusedTitle => Copy.Fields.RefusedTitle;

    /// <summary>False where nothing is ruled out, so the list is all of it.</summary>
    public bool HasRefused { get => _hasRefused; private set => Set(ref _hasRefused, value); }

    /// <summary>Entries the disclosure covers. Empty where it covers none.</summary>
    public string RefusedCount { get => _refusedCount; private set => Set(ref _refusedCount, value); }

    public Icons RefusedGlyph { get => _refusedGlyph; private set => Set(ref _refusedGlyph, value); }

    public bool IsEnabled { get => _isEnabled; private set => Set(ref _isEnabled, value); }

    public bool HasHelp { get => _hasHelp; private set => Set(ref _hasHelp, value); }

    public bool HasDoc { get => _hasDoc; private set => Set(ref _hasDoc, value); }

    public bool HasReason { get => _hasReason; private set => Set(ref _hasReason, value); }

    public bool HasNote { get => _hasNote; private set => Set(ref _hasNote, value); }

    public bool HasUnit { get => _hasUnit; private set => Set(ref _hasUnit, value); }

    public bool IsText { get => _isText; private set => Set(ref _isText, value); }

    public bool IsNumber { get => _isNumber; private set => Set(ref _isNumber, value); }

    /// <summary>
    /// A number that also carries a ladder, drawn as the typed box and the ladder glued into one control.
    /// Not <see cref="IsNumber"/> as well: both write the one setting, so a renderer drawing both would put two
    /// boxes on screen for one knob.
    /// </summary>
    public bool IsNumberSelect { get => _isNumberSelect; private set => Set(ref _isNumberSelect, value); }

    public bool IsSlider { get => _isSlider; private set => Set(ref _isSlider, value); }

    public bool IsToggle { get => _isToggle; private set => Set(ref _isToggle, value); }

    public bool IsSelect { get => _isSelect; private set => Set(ref _isSelect, value); }

    public bool IsRadio { get => _isRadio; private set => Set(ref _isRadio, value); }

    /// <summary>
    /// Either of the two kinds whose whole control is its options.
    /// The generic renderer draws one list for both rather than two lists differing in nothing.
    /// A number carrying a ladder is neither: its options sit behind a caret beside a box, which is a different
    /// control and not a differently spaced list.
    /// </summary>
    public bool IsChoice { get => _isChoice; private set => Set(ref _isChoice, value); }

    public bool IsReadonly { get => _isReadonly; private set => Set(ref _isReadonly, value); }

    /// <summary>The range the form stated, in the type a slider binds. Widest the widget holds where the form stated none.</summary>
    public double Minimum { get => _minimum; private set => Set(ref _minimum, value); }

    public double Maximum { get => _maximum; private set => Set(ref _maximum, value); }

    public double Step { get => _step; private set => Set(ref _step, value); }

    /// <summary>
    /// The same bounds in the type a spinner binds.
    /// Stated twice rather than converted at the binding site, since a compiled binding that has to convert is
    /// one the compiler cannot check.
    /// </summary>
    public decimal NumberMinimum { get => _numberMinimum; private set => Set(ref _numberMinimum, value); }

    public decimal NumberMaximum { get => _numberMaximum; private set => Set(ref _numberMaximum, value); }

    public decimal NumberStep { get => _numberStep; private set => Set(ref _numberStep, value); }

    /// <summary>
    /// The one render function.
    /// Safe to run twice: every output is read out of the message, the options compare equal across two passes
    /// over one field, and the inputs are assigned under the echo guard, so an unchanged form reports nothing
    /// back.
    /// </summary>
    /// <param name="action">
    /// The effect this screen offers beside the control, null where it offers none.
    /// Passed per pass rather than held, so a screen that stops offering it turns the button off through the
    /// render function that turned it on.
    /// </param>
    public void Apply(Field field, Vocabulary words, FieldAction? action = null)
    {
        Assert.NotNull(field, "rendering a field needs the field the form described");
        Assert.NotNull(words, "naming a field's entries needs the vocabulary that names them");
        Assert.That(field.Key == Key, "a field renders the settings field it was made for", Key, field.Key);

        _field = field;
        _words = words;

        Action = action;
        HasAction = action is not null;
        ActionNotice = action?.Notice ?? "";
        HasActionNotice = ActionNotice.Length > 0;

        // Heading and paragraph are keyed by the field the backend named; the reason and the note are codes it
        // sent, turned into sentences here.
        var copy = Copy.Fields.Of(Key);
        Label = copy.Label;
        Help = copy.Help;
        Doc = copy.Doc;
        Unit = Copy.Fields.Unit(field.Unit);
        Reason = Statements.Of(field.Reason);
        Note = Statements.Of(field.Note);
        HasHelp = Help.Length > 0;
        HasDoc = Doc.Length > 0;
        HasReason = Reason.Length > 0;
        HasNote = Note.Length > 0;
        HasUnit = Unit.Length > 0;
        IsVisible = field.Visible;
        IsEnabled = field.Enabled;
        AppliesLive = field.Live;

        IsText = field.Control == ControlKind.Text;
        IsNumber = field.Control == ControlKind.Number;
        IsNumberSelect = field.Control == ControlKind.NumberSelect;
        IsSlider = field.Control == ControlKind.Slider;
        IsToggle = field.Control == ControlKind.Toggle;
        IsSelect = field.Control == ControlKind.Select;
        IsRadio = field.Control == ControlKind.Radio;
        IsChoice = IsSelect || IsRadio;
        IsReadonly = field.Control == ControlKind.Readonly;

        // No range means unbounded, not zero, so a field carrying none takes the widest bounds the widget holds
        // instead of being pinned at nothing.
        Minimum = field.Range?.Min ?? int.MinValue;
        Maximum = field.Range?.Max ?? int.MaxValue;
        Step = field.Range is { Step: > 0 } range ? range.Step : 1;
        NumberMinimum = (decimal)Minimum;
        NumberMaximum = (decimal)Maximum;
        NumberStep = (decimal)Step;

        _kind = field.Value.KindCase;
        Adopt(field.Value);
        Reconcile.Onto(Options, OptionRows(field));

        // Written on every pass, so a control whose last refused entry became reachable stops drawing a
        // disclosure over nothing.
        var refused = Options.Count(option => !option.IsEnabled);
        HasRefused = refused > 0;
        RefusedCount = refused > 0 ? Copy.Fields.RefusedCount(refused) : "";
        RefusedGlyph = RefusedShown ? Icons.IconChevronDown : Icons.IconChevronRight;
        Reconcile.Onto(Shown, ShownRows());
        Reconcile.Onto(MenuRows, MenuRowsOf());

        // The closed dropdown's face, written on every pass including the branch where nothing is picked, so a
        // field whose entry went away cannot go on showing the last one it had.
        var picked = Options.FirstOrDefault(option => option.IsSelected);
        PickedLabel = picked?.Label ?? Readback;
        PickedNote = picked?.Note ?? "";
        HasPickedNote = PickedNote.Length > 0;

        Assert.That(
            Options.Count == 0 || IsSelect || IsRadio || IsNumberSelect,
            "only a control that offers entries carries options", Key, field.Control, Options.Count);
        Assert.That(IsEnabled || HasReason, "a disabled field states why", Key);
        Assert.That(HasAction == (Action is not null), "the action and the flag that draws it agree", Key);
        Assert.That(!HasActionNotice || HasAction, "a sentence about an effect has an effect to be about", Key);
        Assert.That(HasRefused == (RefusedCount.Length > 0), "a disclosure counts what it covers", Key, RefusedCount);
        Assert.That(
            Shown.Count == (RefusedShown ? Options.Count : Options.Count - refused),
            "a list draws every entry it is not hiding", Key, Shown.Count, Options.Count, refused);
        Assert.That(
            MenuRows.Count == Shown.Count + (HasRefused ? 1 : 0),
            "a dropdown adds the disclosure to what a list draws", Key, MenuRows.Count, Shown.Count);
    }

    /// <summary>
    /// Takes the value the form carries, repairs included.
    /// Assigned rather than compared against what the widget held, because adopting the whole draft is what
    /// keeps a greyed option and its replacement from disagreeing.
    /// </summary>
    private void Adopt(FieldValue value)
    {
        _adopting = true;
        try
        {
            switch (value.KindCase)
            {
                case FieldValue.KindOneofCase.Text:
                    Text = value.Text;
                    Readback = value.Text;
                    break;
                case FieldValue.KindOneofCase.Number:
                    Number = value.Number;
                    Slide = value.Number;
                    Readback = value.Number.ToString(CultureInfo.InvariantCulture);
                    break;
                case FieldValue.KindOneofCase.Decimal:
                    Number = (decimal)value.Decimal;
                    Slide = value.Decimal;
                    Readback = value.Decimal.ToString("0.##", CultureInfo.InvariantCulture);
                    break;
                case FieldValue.KindOneofCase.Flag:
                    Flag = value.Flag;
                    Readback = value.Flag ? "on" : "off";
                    break;
                default:
                    Assert.Never("a form field carries a value", Key, value.KindCase);
                    break;
            }
        }
        finally
        {
            _adopting = false;
        }
    }

    /// <summary>
    /// The entries, with the picked one marked.
    /// The command per value is made once and reused, which is what lets an unchanged pass produce rows that
    /// compare equal.
    /// A greyed entry keeps its place: the backend already sank it to the bottom of the list
    /// (docs/field-availability.md, "Where a greyed entry sits").
    /// </summary>
    private IReadOnlyList<OptionViewModel> OptionRows(Field field)
    {
        var picked = FieldValues.AsText(field.Value);

        return field.Options.Select(option => new OptionViewModel
        {
            Value = option.Value,
            Label = _words.Name(Key, option.Value),
            Note = Statements.Of(option.Note),
            Detail = _words.Describe(Key, option.Value),
            IsSelected = option.Value == picked,
            IsEnabled = option.Enabled,
            Reason = Statements.Of(option.Reason),
            IsRecommended = option.Recommended,
            IsReveal = false,
            Choose = ChooseCommand(option.Value),
        }).ToList();
    }

    /// <summary>The entries a list draws: the reachable ones, and the rest below them once the reader has asked.</summary>
    private IReadOnlyList<OptionViewModel> ShownRows()
    {
        var offered = Options.Where(option => option.IsEnabled).ToList();
        if (!RefusedShown)
        {
            return offered;
        }

        offered.AddRange(Options.Where(option => !option.IsEnabled));
        return offered;
    }

    /// <summary><see cref="Shown"/> with the disclosure last, for the surfaces whose list is a popup.</summary>
    private IReadOnlyList<OptionViewModel> MenuRowsOf()
    {
        if (!HasRefused)
        {
            return Shown;
        }

        var rows = new List<OptionViewModel>(Shown.Count + 1);
        rows.AddRange(Shown);
        rows.Add(new OptionViewModel
        {
            Value = "",
            Label = RefusedTitle,
            Note = RefusedCount,
            Detail = "",
            IsSelected = RefusedShown,
            IsEnabled = true,
            Reason = "",
            IsRecommended = false,
            IsReveal = true,
            Choose = RevealCommand,
        });

        return rows;
    }

    /// <summary>The one departure from idempotency here, and the verb says so: two presses are two states.</summary>
    private void ToggleRefused() => RefusedShown = !RefusedShown;

    private DelegateCommand ChooseCommand(string value)
    {
        if (_choose.TryGetValue(value, out var command))
        {
            return command;
        }

        // The kind is read when the command runs rather than captured here, so a command made on one pass still
        // writes the type the field carries on the next.
        command = new DelegateCommand(() => _write(Key, FieldValues.Of(_kind, value)));
        _choose[value] = command;
        return command;
    }
}
