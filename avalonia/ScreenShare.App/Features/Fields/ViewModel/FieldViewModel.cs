using System.Collections.ObjectModel;
using System.Globalization;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Fields.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Fields.ViewModel;

/// <summary>
/// One control of the resolved form, as the screen draws it.
///
/// This is the shell's whole understanding of a setting: a key, a control kind it maps to
/// a widget, and a handful of statements the backend made about it. It evaluates no rule
/// and invents no value - which entries exist, which are reachable and why not all arrive
/// already decided (docs/ipc-api.md, "The rule").
///
/// What it does own is every word. The heading, the paragraph behind it, what each entry
/// is called and the sentence in place of a greyed one are all written on this side and
/// looked up by <see cref="Key"/> and by the entry's value (ScreenShare.App.Copy). The
/// backend sends <c>hevc_nvenc</c>; this is where that becomes something to read.
///
/// <b>Inputs</b> are <see cref="Text"/>, <see cref="Number"/>, <see cref="Slide"/> and
/// <see cref="Flag"/>, the four shapes a widget writes back. Each setter reports the change
/// to whoever owns the draft and nothing else; it does not decide whether the change was
/// legal, because the next resolved form is the answer to that.
///
/// <b>Outputs</b> are written by <see cref="Apply"/> on every pass, including the branches
/// that turn a control off. The one departure from the usual split is deliberate: the
/// value is an output as well, since the backend may repair a draft and a shell adopts the
/// repaired one wholesale. The echo guard is what keeps that from being read back as a
/// fresh edit.
/// </summary>
public sealed class FieldViewModel : Observable
{
    private readonly Action<string, FieldValue> _write;
    private readonly Dictionary<string, DelegateCommand> _choose = [];

    /// <summary>
    /// Which shape this field's value has, as the last resolved form carried it. An option
    /// value is a string whatever the field is, so this is what turns a pick back into the
    /// type the settings field holds - without which a select over a number would mark one
    /// entry and write zero.
    /// </summary>
    private FieldValue.KindOneofCase _kind = FieldValue.KindOneofCase.Text;

    /// <summary>
    /// True while <see cref="Apply"/> is assigning the inputs. A widget's setter checks it
    /// and reports nothing, so adopting a repaired value does not read as the user typing.
    /// </summary>
    private bool _adopting;

    /// <summary>
    /// How this field's entries are named, as the last pass was given it. It is held rather
    /// than passed down because the entries are rebuilt inside the same pass that sets it,
    /// and a naming that lagged one pass behind would label a fresh list from a stale
    /// catalog.
    /// </summary>
    private Vocabulary _words = Vocabulary.Empty;

    public FieldViewModel(string key, Action<string, FieldValue> write)
    {
        Assert.That(key.Length > 0, "a field is identified by the settings field it edits");
        Assert.NotNull(write, "a field needs somewhere to report what the user moved");

        Key = key;
        _write = write;
        Options = [];
    }

    /// <summary>The settings field this control edits. Carried, never parsed.</summary>
    public string Key { get; }

    // --- Inputs -------------------------------------------------------------------

    private string _text = "";
    private decimal? _number;
    private double _slide;
    private bool _flag;

    /// <summary>A free-text value, as a text box holds it.</summary>
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

    /// <summary>A typed number, as a spinner holds it. Null while the box is empty, which reports nothing.</summary>
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

    /// <summary>A swept number, as a slider holds it. The same settings value as <see cref="Number"/>, in the type a slider binds.</summary>
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

    /// <summary>The entries of a select or radio, empty on every other kind.</summary>
    public ObservableCollection<OptionViewModel> Options { get; }

    /// <summary>
    /// The effect offered beside this control, null where the screen offers none. It is the
    /// screen's own placement rather than anything the form described, and it writes this field
    /// through the same path a keystroke does (<see cref="FieldAction"/>).
    /// </summary>
    public FieldAction? Action { get => _action; private set => Set(ref _action, value); }

    public bool HasAction { get => _hasAction; private set => Set(ref _hasAction, value); }

    /// <summary>
    /// Why the effect beside this control is refused, or what its last attempt answered. Empty
    /// where there is no effect or nothing to say about it.
    ///
    /// It is lifted off the action rather than bound through it, because a control that offers
    /// none has no action to bind through - and a binding down a null path draws the sentence's
    /// absence as an empty line instead of no line.
    /// </summary>
    public string ActionNotice { get => _actionNotice; private set => Set(ref _actionNotice, value); }

    public bool HasActionNotice { get => _hasActionNotice; private set => Set(ref _hasActionNotice, value); }

    public string Label { get => _label; private set => Set(ref _label, value); }

    /// <summary>The prose behind the control. The form is pedagogical, and this is where it teaches.</summary>
    public string Help { get => _help; private set => Set(ref _help, value); }

    /// <summary>The reference article for the concept, empty where this control has none.</summary>
    public string Doc { get => _doc; private set => Set(ref _doc, value); }

    public string Unit { get => _unit; private set => Set(ref _unit, value); }

    /// <summary>Why the control is inert, shown in its place. Empty while it is live.</summary>
    public string Reason { get => _reason; private set => Set(ref _reason, value); }

    /// <summary>What the field means here that its label does not say. Carried by a live field.</summary>
    public string Note { get => _note; private set => Set(ref _note, value); }

    /// <summary>The value as a read-back prints it, for a field with no input on it.</summary>
    public string Readback { get => _readback; private set => Set(ref _readback, value); }

    /// <summary>
    /// The picked entry's label, which is what a closed dropdown shows. It falls back to the
    /// raw value for the case the contract allows: a backend may answer with a legal value
    /// that is not one of the entries it offered.
    /// </summary>
    public string PickedLabel { get => _pickedLabel; private set => Set(ref _pickedLabel, value); }

    /// <summary>The picked entry's trailing note, which is what makes a closed dropdown honest.</summary>
    public string PickedNote { get => _pickedNote; private set => Set(ref _pickedNote, value); }

    public bool HasPickedNote { get => _hasPickedNote; private set => Set(ref _hasPickedNote, value); }

    /// <summary>False for a knob with no meaning outside one selection. The control is not drawn at all.</summary>
    public bool IsVisible { get => _isVisible; private set => Set(ref _isVisible, value); }

    public bool IsEnabled { get => _isEnabled; private set => Set(ref _isEnabled, value); }

    public bool HasHelp { get => _hasHelp; private set => Set(ref _hasHelp, value); }

    public bool HasDoc { get => _hasDoc; private set => Set(ref _hasDoc, value); }

    public bool HasReason { get => _hasReason; private set => Set(ref _hasReason, value); }

    public bool HasNote { get => _hasNote; private set => Set(ref _hasNote, value); }

    public bool HasUnit { get => _hasUnit; private set => Set(ref _hasUnit, value); }

    public bool IsText { get => _isText; private set => Set(ref _isText, value); }

    public bool IsNumber { get => _isNumber; private set => Set(ref _isNumber, value); }

    /// <summary>
    /// A number that also carries a ladder, drawn as the typed box and the ladder glued into
    /// one control. It is not <see cref="IsNumber"/>: the two write the same setting, so a
    /// renderer that drew both would put two boxes on the screen for one knob.
    /// </summary>
    public bool IsNumberSelect { get => _isNumberSelect; private set => Set(ref _isNumberSelect, value); }

    public bool IsSlider { get => _isSlider; private set => Set(ref _isSlider, value); }

    public bool IsToggle { get => _isToggle; private set => Set(ref _isToggle, value); }

    public bool IsSelect { get => _isSelect; private set => Set(ref _isSelect, value); }

    public bool IsRadio { get => _isRadio; private set => Set(ref _isRadio, value); }

    /// <summary>
    /// Either of the two kinds whose whole control is its options. The generic renderer draws
    /// one list for both, so it asks this rather than binding two lists that differed in
    /// nothing. A number carrying a ladder is not one of them: its options sit behind a caret
    /// beside a box, which is a different control and not a differently spaced list.
    /// </summary>
    public bool IsChoice { get => _isChoice; private set => Set(ref _isChoice, value); }

    public bool IsReadonly { get => _isReadonly; private set => Set(ref _isReadonly, value); }

    /// <summary>The slider's bounds, in the type a slider binds.</summary>
    public double Minimum { get => _minimum; private set => Set(ref _minimum, value); }

    public double Maximum { get => _maximum; private set => Set(ref _maximum, value); }

    public double Step { get => _step; private set => Set(ref _step, value); }

    /// <summary>
    /// The same bounds in the type a spinner binds. Stated twice rather than converted at
    /// the binding, because a compiled binding that has to convert is one the compiler
    /// cannot check.
    /// </summary>
    public decimal NumberMinimum { get => _numberMinimum; private set => Set(ref _numberMinimum, value); }

    public decimal NumberMaximum { get => _numberMaximum; private set => Set(ref _numberMaximum, value); }

    public decimal NumberStep { get => _numberStep; private set => Set(ref _numberStep, value); }

    /// <summary>
    /// The one render function. Safe to run twice: every output is read out of the message,
    /// the options compare equal across two passes over one field, and the inputs are
    /// assigned under the echo guard so an unchanged form reports nothing back.
    /// </summary>
    /// <param name="action">
    /// The effect this screen offers beside the control, null where it offers none. Passed on
    /// every pass rather than held, so a screen that stops offering it turns the button off
    /// through the same render function that turned it on.
    /// </param>
    public void Apply(Field field, Vocabulary words, FieldAction? action = null)
    {
        Assert.NotNull(field, "rendering a field needs the field the form described");
        Assert.NotNull(words, "naming a field's entries needs the vocabulary that names them");
        Assert.That(field.Key == Key, "a field renders the settings field it was made for", Key, field.Key);

        _words = words;

        Action = action;
        HasAction = action is not null;
        ActionNotice = action?.Notice ?? "";
        HasActionNotice = ActionNotice.Length > 0;

        // The heading and the paragraph are this side's, keyed by the field the backend
        // named; the reason and the note are statements it made, turned into sentences here.
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

        IsText = field.Control == ControlKind.Text;
        IsNumber = field.Control == ControlKind.Number;
        IsNumberSelect = field.Control == ControlKind.NumberSelect;
        IsSlider = field.Control == ControlKind.Slider;
        IsToggle = field.Control == ControlKind.Toggle;
        IsSelect = field.Control == ControlKind.Select;
        IsRadio = field.Control == ControlKind.Radio;
        IsChoice = IsSelect || IsRadio;
        IsReadonly = field.Control == ControlKind.Readonly;

        // Absence of a range means unbounded rather than zero, so a field with none is given
        // the widest bounds the widget can hold instead of being pinned at nothing.
        Minimum = field.Range?.Min ?? int.MinValue;
        Maximum = field.Range?.Max ?? int.MaxValue;
        Step = field.Range is { Step: > 0 } range ? range.Step : 1;
        NumberMinimum = (decimal)Minimum;
        NumberMaximum = (decimal)Maximum;
        NumberStep = (decimal)Step;

        _kind = field.Value.KindCase;
        Adopt(field.Value);
        Reconcile.Onto(Options, OptionRows(field));

        // The closed dropdown's face, set on every pass including the branch where nothing is
        // picked, so a field that lost its entry cannot go on showing the last one it had.
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
    }

    /// <summary>
    /// Takes the value the form carries. It is the backend's answer including any repair, so
    /// it is assigned rather than compared against what the widget held: adopting the whole
    /// draft is what keeps a greyed option and its replacement from disagreeing.
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
    /// The entries, with the picked one marked. The command per value is made once and
    /// reused, which is what lets an unchanged pass produce rows that compare equal.
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
            Choose = ChooseCommand(option.Value),
        }).ToList();
    }

    private DelegateCommand ChooseCommand(string value)
    {
        if (_choose.TryGetValue(value, out var command))
        {
            return command;
        }

        // The kind is read when the command runs rather than captured now, so a command made
        // on one pass still writes the type the field carries on the next.
        command = new DelegateCommand(() => _write(Key, FieldValues.Of(_kind, value)));
        _choose[value] = command;
        return command;
    }
}
