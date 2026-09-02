using System.Collections.ObjectModel;
using System.Globalization;
using Avalonia.Collections;
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
/// Shell's whole understanding of a setting: a key, a control kind mapped to a widget,
/// and the statements the backend made about it.
/// No rule is evaluated and no value invented here; which entries exist, which are reachable
/// and why not arrive already decided (<c>docs/ipc-api.md</c>, "The rule").
///
/// Every word is this side's.
/// The heading, the paragraph behind it, what an entry is called
/// and the sentence standing in for a greyed one are looked up by <see cref="Key"/> and by the entry's value
/// (ScreenShare.App.Copy).
/// The backend sends <c>hevc_nvenc</c>; this is where that becomes something to read.
///
/// Inputs are <see cref="Text"/>, <see cref="Number"/>, <see cref="Slide"/> and <see cref="Flag"/>,
/// the four shapes a widget writes back.
/// A setter reports the change to whoever owns the draft and nothing else,
/// since the next resolved form is the answer to whether the change was legal.
/// <see cref="RefusedShown"/> is the one input reporting nothing: which entries a reader looks at changes no setting,
/// so it re-renders here and goes no further.
///
/// Outputs are written by <see cref="Apply"/> on every pass, including the branches that turn a control off.
/// The value departs from that split and is both: the backend may repair a draft
/// and a shell adopts the repaired one wholesale, so <see cref="Apply"/> assigns the inputs too.
/// The echo guard keeps an adopted value from reading back as a fresh edit.
/// </summary>
public sealed class FieldViewModel : Observable
{
    private readonly Action<string, FieldValue> _write;

    /// <summary>
    /// Reports that the reader has this control's thumb and has not let go.
    /// The one fact about a gesture that leaves this layer,
    /// and it leaves because a repair answered under a moving thumb waits for the release
    /// (<see cref="IsSweeping"/>).
    /// </summary>
    private readonly Action<bool> _sweep;

    private readonly Dictionary<string, DelegateCommand> _choose = [];

    /// <summary>
    /// Shape of this field's value, as the last resolved form carried it.
    /// An option value crosses as a string whatever the field is,
    /// so this turns a pick back into the type the settings field holds;
    /// without it a select over a number would mark an entry and write zero.
    /// </summary>
    private FieldValue.KindOneofCase _kind = FieldValue.KindOneofCase.Text;

    /// <summary>
    /// True while <see cref="Apply"/> is assigning the inputs.
    /// A widget's setter checks it and reports nothing, so adopting a repaired value does not read as typing.
    /// </summary>
    private bool _adopting;

    /// <summary>
    /// How this field's entries are named, as the last pass was given it.
    /// Held rather than passed down because the entries are rebuilt inside the same pass that sets it,
    /// and a naming one pass behind would label a fresh list from a stale catalog.
    /// </summary>
    private Vocabulary _words = Vocabulary.Empty;

    /// <summary>
    /// Last field a pass was given,
    /// so revealing the refused entries renders from what the backend said rather than from what the widgets hold.
    /// </summary>
    private Field? _field;

    /// <summary>
    /// Whether that field answered for the draft as it stood.
    /// Held with it, so a list opened between a write
    /// and its answer redraws on the same terms the pass did rather than taking a value the reader has moved off.
    /// </summary>
    private bool _answered = true;

    /// <param name="sweep">
    /// Takes the two edges of a gesture on this control, null where the screen has nowhere to report them.
    /// A group given none draws controls that ask about every value the thumb passes over.
    /// </param>
    public FieldViewModel(string key, Action<string, FieldValue> write, Action<bool>? sweep = null)
    {
        Assert.That(key.Length > 0, "a field is identified by the settings field it edits");
        Assert.NotNull(write, "a field needs somewhere to report what the user moved");

        Key = key;
        _write = write;
        _sweep = sweep ?? (_ => { });
        Options = [];
        Shown = [];
        MenuRows = [];
        RevealCommand = new DelegateCommand(ToggleRefused);
    }

    /// <summary>The settings field this control edits, as <c>publish.encoder</c>. Carried, never parsed.</summary>
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
            if (!Set(ref _text, value))
            {
                return;
            }

            Readback = value;
            if (!_adopting)
            {
                _write(Key, new FieldValue { Text = value });
            }
        }
    }

    /// <summary>
    /// Typed number, in the type a spinner binds.
    /// Null while the box is empty, and an empty box reports nothing.
    /// </summary>
    public decimal? Number
    {
        get => _number;
        set
        {
            if (!Set(ref _number, value) || value is not { } number)
            {
                return;
            }

            Readback = Printed((double)number);
            if (!_adopting)
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
            if (!Set(ref _slide, value))
            {
                return;
            }

            Readback = Printed(value);
            if (!_adopting)
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
            if (!Set(ref _flag, value))
            {
                return;
            }

            Readback = value ? "on" : "off";
            if (!_adopting)
            {
                _write(Key, new FieldValue { Flag = value });
            }
        }
    }

    private bool _isSweeping;

    /// <summary>
    /// True while the reader is holding this control's thumb.
    /// Written by the widget, a gesture being the view's to know and nobody else's (<c>Controls/FieldSlider</c>).
    ///
    /// What it buys: a repair answered mid-gesture is adopted on the release rather than under the pointer
    /// (<c>docs/settings-editing.md</c>).
    /// Every value the thumb passes over is asked about, so the price and the greyings follow the gesture,
    /// one round trip behind.
    /// The number beside the control follows the thumb itself,
    /// printed from what this holds rather than from the last answer.
    /// </summary>
    public bool IsSweeping
    {
        get => _isSweeping;
        set
        {
            if (Set(ref _isSweeping, value))
            {
                _sweep(value);
            }
        }
    }

    private bool _refusedShown;

    /// <summary>
    /// Held here and not in the view: a list the reader opened is state the reader set,
    /// and the same answer has to reach a card list and a dropdown's rows alike.
    /// </summary>
    public bool RefusedShown
    {
        get => _refusedShown;
        set
        {
            if (Set(ref _refusedShown, value) && _field is not null)
            {
                Apply(_field, _words, Action, _answered);
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
    private bool _isEntryOutsideBand;
    private bool _isNumberBox;
    private bool _isSlider;
    private bool _isToggle;
    private bool _isSelect;
    private bool _isRadio;
    private bool _isChoice;
    private bool _isReadonly;
    private double _minimum;
    private double _maximum;
    private double _step = 1;
    private double _pageStep = 10;
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
    /// Entries of a select, a radio or a number carrying a ladder.
    /// Empty on every other kind.
    /// Every one of them, refused included, in the order the form gave them;
    /// what a surface draws is <see cref="Shown"/> or <see cref="MenuRows"/>.
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
    /// Effect offered beside this control, null where the screen offers none.
    /// The screen's own placement rather than anything the form described,
    /// and it writes this field through the path a keystroke takes (<see cref="FieldAction"/>).
    /// </summary>
    public FieldAction? Action { get => _action; private set => Set(ref _action, value); }

    public bool HasAction { get => _hasAction; private set => Set(ref _hasAction, value); }

    /// <summary>
    /// Why the effect beside this control is refused, or what its last attempt answered.
    /// Empty where there is no effect, or nothing to say about one.
    /// Lifted off the action rather than bound through it: a control offering none has no action to bind through,
    /// and a binding down a null path draws the absent sentence as an empty line instead of no line.
    /// </summary>
    public string ActionNotice { get => _actionNotice; private set => Set(ref _actionNotice, value); }

    public bool HasActionNotice { get => _hasActionNotice; private set => Set(ref _hasActionNotice, value); }

    public string Label { get => _label; private set => Set(ref _label, value); }

    /// <summary>Paragraph behind the control. The form is pedagogical, and this is where it teaches.</summary>
    public string Help { get => _help; private set => Set(ref _help, value); }

    /// <summary>Reference article for the concept, empty where the control has none.</summary>
    public string Doc { get => _doc; private set => Set(ref _doc, value); }

    public string Unit { get => _unit; private set => Set(ref _unit, value); }

    /// <summary>Why the control is inert, drawn in its place. Empty while it is live.</summary>
    public string Reason { get => _reason; private set => Set(ref _reason, value); }

    /// <summary>
    /// What the field means here that its label does not say.
    /// The control stays editable, so a note takes nothing away.
    /// </summary>
    public string Note { get => _note; private set => Set(ref _note, value); }

    /// <summary>
    /// Value as a read-back prints it: the figure beside a slider, and the whole of a field with no input on it.
    ///
    /// Printed from what the control holds rather than from the last answer,
    /// so it is the number under the thumb while a sweep is on,
    /// and the resolve confirming it has not been asked for yet.
    /// A repair still lands here, arriving through the inputs the answer assigns.
    /// </summary>
    public string Readback { get => _readback; private set => Set(ref _readback, value); }

    /// <summary>
    /// Picked entry's label, drawn on a closed dropdown.
    /// Falls back to the raw value for the case the contract allows: a legal value that is none of the entries offered.
    /// </summary>
    public string PickedLabel { get => _pickedLabel; private set => Set(ref _pickedLabel, value); }

    /// <summary>Picked entry's trailing note, what makes a closed dropdown honest.</summary>
    public string PickedNote { get => _pickedNote; private set => Set(ref _pickedNote, value); }

    public bool HasPickedNote { get => _hasPickedNote; private set => Set(ref _hasPickedNote, value); }

    /// <summary>False for a knob with no meaning outside one selection. The control is not drawn at all.</summary>
    public bool IsVisible { get => _isVisible; private set => Set(ref _isVisible, value); }

    /// <summary>
    /// True where the value is written to the pipeline already carrying the stream,
    /// so an edit costs nobody watching a reconnect.
    /// False where applying it replaces the encoder child and every viewer reconnects across the gap.
    ///
    /// Backend's answer per combination rather than a list held here:
    /// the engine behind the capture backend decides whether anything is live at all, and the codec
    /// and rate-control mode decide whether the encoder is being sent that value.
    /// A list on this side would go on promising a reconnect-free edit after the backend stopped delivering one
    /// (<c>docs/field-availability.md</c>, "A live stream blocks no field").
    /// </summary>
    public bool AppliesLive { get => _appliesLive; private set => Set(ref _appliesLive, value); }

    /// <summary>What changing an <see cref="AppliesLive"/> control costs,
    /// in the width a chip beside a label has.</summary>
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
    /// Not <see cref="IsNumber"/> as well: both write the one setting,
    /// so a renderer drawing both would put two boxes on screen for one knob.
    /// </summary>
    public bool IsNumberSelect { get => _isNumberSelect; private set => Set(ref _isNumberSelect, value); }

    /// <summary>
    /// A number-select resting on an entry its own range does not reach, drawn as that entry with the ladder beside it
    /// and no box to type in.
    /// </summary>
    /// <remarks>
    /// The contract allows an entry outside the range, for the control whose legal values are a band
    /// and a value the band cannot hold: the burst ceiling is bounded by nothing at zero
    /// and legal again from the target it bursts above (form.proto, CONTROL_KIND_NUMBER_SELECT).
    /// A spinner handed such a value coerces it to its own floor and writes the floor back through the binding,
    /// which replaces an uncapped burst with a capped one nobody asked for.
    /// Drawing the entry instead is what keeps the held answer held,
    /// and the box returns with the next value inside the band.
    /// </remarks>
    public bool IsEntryOutsideBand { get => _isEntryOutsideBand; private set => Set(ref _isEntryOutsideBand, value); }

    /// <summary>
    /// A number-select drawing its typed box, which is every one of them
    /// but the one resting on an entry its range does not reach.
    /// </summary>
    public bool IsNumberBox { get => _isNumberBox; private set => Set(ref _isNumberBox, value); }

    public bool IsSlider { get => _isSlider; private set => Set(ref _isSlider, value); }

    public bool IsToggle { get => _isToggle; private set => Set(ref _isToggle, value); }

    public bool IsSelect { get => _isSelect; private set => Set(ref _isSelect, value); }

    public bool IsRadio { get => _isRadio; private set => Set(ref _isRadio, value); }

    /// <summary>
    /// Either of the two kinds whose whole control is its options.
    /// The generic renderer draws one list for both rather than two lists differing in nothing.
    /// A number carrying a ladder is neither: its options sit behind a caret beside a box, which is a different control
    /// and not a differently spaced list.
    /// </summary>
    public bool IsChoice { get => _isChoice; private set => Set(ref _isChoice, value); }

    public bool IsReadonly { get => _isReadonly; private set => Set(ref _isReadonly, value); }

    /// <summary>
    /// Range the form stated, in the type a slider binds.
    /// Widest the widget holds where the form stated none.
    /// </summary>
    public double Minimum { get => _minimum; private set => Set(ref _minimum, value); }

    public double Maximum { get => _maximum; private set => Set(ref _maximum, value); }

    /// <summary>Distance between the values a slider stops on, and what an arrow key moves by.</summary>
    public double Step { get => _step; private set => Set(ref _step, value); }

    /// <summary>
    /// What a page key moves by: ten steps, so a range a step crosses in a hundred presses takes ten.
    /// A multiple of the step, or a page key would land between two stops
    /// and the slider would snap back to the one it started on.
    /// </summary>
    public double PageStep { get => _pageStep; private set => Set(ref _pageStep, value); }

    /// <summary>
    /// Stops between the ends: the multiples of <see cref="Step"/> the range contains.
    /// Round figures rather than a grid counted off the floor, so a 20 ms floor with a 50 ms step stops on 20, 50, 100
    /// and not on 20, 70, 120.
    ///
    /// The ends are left out, a slider stopping on <see cref="Minimum"/>
    /// and <see cref="Maximum"/> whatever this list holds,
    /// which keeps the shortest window reachable under a step that steps straight over it.
    /// Mutated in place: the widget reads the list as it stands,
    /// and a fresh instance per pass would rebind the control on every render.
    /// </summary>
    public AvaloniaList<double> Ticks { get; } = [];

    /// <summary>
    /// Same bounds in the type a spinner binds.
    /// Stated twice rather than converted at the binding site,
    /// a compiled binding that has to convert being one the compiler cannot check.
    /// </summary>
    public decimal NumberMinimum { get => _numberMinimum; private set => Set(ref _numberMinimum, value); }

    public decimal NumberMaximum { get => _numberMaximum; private set => Set(ref _numberMaximum, value); }

    public decimal NumberStep { get => _numberStep; private set => Set(ref _numberStep, value); }

    /// <summary>
    /// One render function.
    /// Safe to run twice: every output is read out of the message,
    /// the options compare equal across two passes over one field, and the inputs are assigned under the echo guard,
    /// so an unchanged form reports nothing back.
    /// </summary>
    /// <param name="action">
    /// Effect this screen offers beside the control, null where it offers none.
    /// Passed per pass rather than held,
    /// so a screen that stops offering it turns the button off through the render function that turned it on.
    /// </param>
    /// <param name="answered">
    /// Whether this form answers for the draft as it now stands (<see cref="FormSession.IsAnswered"/>).
    /// False leaves the control holding what the reader put there and draws everything else.
    /// </param>
    public void Apply(Field field, Vocabulary words, FieldAction? action = null, bool answered = true)
    {
        Assert.NotNull(field, "rendering a field needs the field the form described");
        Assert.NotNull(words, "naming a field's entries needs the vocabulary that names them");
        Assert.That(field.Key == Key, "a field renders the settings field it was made for", Key, field.Key);
        Assert.That(field.Control != ControlKind.Slider || field.Range is not null,
            "a slider states the range it sweeps, there being no thumb to place without both ends", Key);

        _field = field;
        _words = words;
        _answered = answered;

        Action = action;
        HasAction = action is not null;
        ActionNotice = action?.Notice ?? "";
        HasActionNotice = ActionNotice.Length > 0;

        // Heading and paragraph are keyed by the field the backend named; the reason and the note are codes it sent,
        // turned into sentences here.
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

        // No range means unbounded, not zero,
        // so a field carrying none takes the widest bounds the widget holds instead of being pinned at nothing.
        Minimum = field.Range?.Min ?? int.MinValue;
        Maximum = field.Range?.Max ?? int.MaxValue;
        Step = field.Range is { Step: > 0 } range ? range.Step : 1;
        PageStep = Step * 10;
        Reconcile.Onto(Ticks, IsSlider ? StopsInside() : []);
        NumberMinimum = (decimal)Minimum;
        NumberMaximum = (decimal)Maximum;
        NumberStep = (decimal)Step;

        _kind = field.Value.KindCase;

        // The one output a pass may leave alone:
        // a form answering for a draft the reader has moved off carries the value they held a round trip ago,
        // and assigning it would pull the thumb back under the pointer on every step of a sweep.
        // The answer about what they hold now is what lands here instead, repairs and all.
        if (answered)
        {
            Adopt(field.Value);
        }

        Reconcile.Onto(Options, OptionRows(field));

        // Written on every pass,
        // so a control whose last refused entry became reachable stops drawing a disclosure over nothing.
        var refused = Options.Count(option => !option.IsEnabled);
        HasRefused = refused > 0;
        RefusedCount = refused > 0 ? Copy.Fields.RefusedCount(refused) : "";
        RefusedGlyph = RefusedShown ? Icons.IconChevronDown : Icons.IconChevronRight;
        Reconcile.Onto(Shown, ShownRows());
        Reconcile.Onto(MenuRows, MenuRowsOf());

        // The closed dropdown's face, written on every pass including the branch where nothing is picked,
        // so a field whose entry went away cannot go on showing the last one it had.
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
        // Written after the entries, the answer being whether the picked one sits inside the range.
        IsEntryOutsideBand = IsNumberSelect
            && field.Range is not null
            && Number is { } held
            && ((long)held < field.Range.Min || (long)held > field.Range.Max)
            && Options.Any(option => option.IsSelected);
        IsNumberBox = IsNumberSelect && !IsEntryOutsideBand;

        Assert.That(HasRefused == (RefusedCount.Length > 0), "a disclosure counts what it covers", Key, RefusedCount);
        Assert.That(
            Shown.Count == (RefusedShown ? Options.Count : Options.Count - refused),
            "a list draws every entry it is not hiding", Key, Shown.Count, Options.Count, refused);
        Assert.That(
            MenuRows.Count == Shown.Count + (HasRefused ? 1 : 0),
            "a dropdown adds the disclosure to what a list draws", Key, MenuRows.Count, Shown.Count);
    }

    /// <summary>
    /// Multiples of <see cref="Step"/> the range contains, ends excluded.
    /// Counted off zero rather than off the floor, which puts them on round figures.
    /// </summary>
    private IReadOnlyList<double> StopsInside()
    {
        Assert.That(Step > 0, "a sweep needs a step to count its stops in", Key, Step);
        Assert.That(Minimum <= Maximum, "a range runs upwards", Key, Minimum, Maximum);

        var stops = new List<double>();
        for (var multiple = (long)Math.Ceiling(Minimum / Step); multiple * Step < Maximum; multiple++)
        {
            var stop = multiple * Step;
            if (stop > Minimum)
            {
                stops.Add(stop);
            }
        }

        return stops;
    }

    /// <summary>
    /// Takes the value the form carries, repairs included.
    /// Assigned rather than compared against what the widget held,
    /// adopting the whole draft being what keeps a greyed option and its replacement from disagreeing.
    /// </summary>
    private void Adopt(FieldValue value)
    {
        _adopting = true;
        try
        {
            switch (value.KindCase)
            {
                // The read-back rides the inputs, which the branches below assign,
                // so an answer that repaired a value prints the repaired one
                // and an unchanged pass prints what is already there.
                case FieldValue.KindOneofCase.Text:
                    Text = value.Text;
                    break;
                case FieldValue.KindOneofCase.Number:
                    Number = value.Number;
                    Slide = value.Number;
                    break;
                case FieldValue.KindOneofCase.Decimal:
                    Number = (decimal)value.Decimal;
                    Slide = value.Decimal;
                    break;
                case FieldValue.KindOneofCase.Flag:
                    Flag = value.Flag;
                    break;
                default:
                    Assert.Never("a form field carries a value", Key, value.KindCase);
                    break;
            }

            // Written on every pass rather than left to the setters,
            // an input the answer did not move assigning nothing.
            Readback = value.KindCase switch
            {
                FieldValue.KindOneofCase.Text => value.Text,
                FieldValue.KindOneofCase.Number => Printed(value.Number),
                FieldValue.KindOneofCase.Decimal => Printed(value.Decimal),
                FieldValue.KindOneofCase.Flag => value.Flag ? "on" : "off",
                _ => Readback,
            };
        }
        finally
        {
            _adopting = false;
        }
    }

    /// <summary>
    /// One number as the read-back prints it, on the scale this field's value carries: 1200,
    /// and 1.25 where the value is a decimal.
    /// </summary>
    private string Printed(double number) => _kind == FieldValue.KindOneofCase.Decimal
        ? number.ToString("0.##", CultureInfo.InvariantCulture)
        : ((long)Math.Round(number)).ToString(CultureInfo.InvariantCulture);

    /// <summary>
    /// Entries, with the picked one marked.
    /// The command per value is made once and reused, so an unchanged pass produces rows that compare equal.
    /// A greyed entry keeps its place: the backend already sank it to the bottom of the list
    /// (<c>docs/field-availability.md</c>, "Where a greyed entry sits").
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

    /// <summary>Entries a list draws: the reachable ones, and the rest below them once the reader has asked.</summary>
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

    /// <summary>One departure from idempotency here, and the verb says so: two presses are two states.</summary>
    private void ToggleRefused() => RefusedShown = !RefusedShown;

    private DelegateCommand ChooseCommand(string value)
    {
        if (_choose.TryGetValue(value, out var command))
        {
            return command;
        }

        // The kind is read when the command runs rather than captured here,
        // so a command made on one pass still writes the type the field carries on the next.
        command = new DelegateCommand(() => _write(Key, FieldValues.Of(_kind, value)));
        _choose[value] = command;
        return command;
    }
}
