using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.Fields.ViewModel;

/// <summary>
/// One group of the resolved form: a heading and the controls under it, in the order the
/// backend gave them.
///
/// This is what makes the five setup steps one component rather than five. Capture, encode,
/// transport, network and destination differ in nothing a shell can see - they are runs of
/// fields with different keys - so a step that hand-wrote its own controls would be five
/// copies of this file, each with its own chance of disagreeing with the form it renders.
///
/// <b>Outputs</b> only. The group has no input of its own: every write goes through a
/// <see cref="FieldViewModel"/> to the flow that owns the draft.
/// </summary>
public sealed class FieldGroupViewModel : Observable
{
    private readonly Action<string, FieldValue> _write;

    /// <summary>
    /// One view model per field key, kept across passes. Fields are widgets with focus and a
    /// caret in them, so they are updated in place; rebuilding them every pass would take
    /// the caret out of a box while it is being typed in.
    /// </summary>
    private readonly Dictionary<string, FieldViewModel> _fields = [];

    public FieldGroupViewModel(Action<string, FieldValue> write)
    {
        Assert.NotNull(write, "a group needs somewhere to report what the user moved");

        _write = write;
        Fields = [];
    }

    // --- Outputs ------------------------------------------------------------------

    private string _title = "";
    private string _help = "";
    private string _summary = "";
    private bool _hasHelp;
    private bool _isResolved;

    /// <summary>The visible controls, in render order. A hidden field is absent rather than collapsed.</summary>
    public ObservableCollection<FieldViewModel> Fields { get; }

    public string Title { get => _title; private set => Set(ref _title, value); }

    public string Help { get => _help; private set => Set(ref _help, value); }

    /// <summary>
    /// What this group settled on, as the step strip repeats it. Composed here out of the
    /// draft the form carried: it is a shorthand, and the separator, the abbreviation and
    /// the length all belong to the strip it sits in.
    /// </summary>
    public string Summary { get => _summary; private set => Set(ref _summary, value); }

    public bool HasHelp { get => _hasHelp; private set => Set(ref _hasHelp, value); }

    /// <summary>
    /// Whether the form carries this group at all. False is the honest state for a step the
    /// backend has not described yet, and the panel says so rather than drawing an empty card.
    /// </summary>
    public bool IsResolved { get => _isResolved; private set => Set(ref _isResolved, value); }

    /// <summary>
    /// The one render function. Safe to run twice: field view models are reused by key and
    /// each runs its own idempotent pass, so an unchanged group produces no notification.
    ///
    /// A null group is the branch that turns everything off, which is what a step whose
    /// group the form did not carry has to render.
    /// </summary>
    public void Apply(FieldGroup? group, Vocabulary words, StreamSettings? settings)
    {
        Assert.NotNull(words, "rendering a group needs the vocabulary that names its entries");

        IsResolved = group is not null;

        // The heading and the sentence under it are this side's, looked up by the key the
        // backend named the group by.
        var copy = group is null ? null : Copy.Fields.Group(group.Key);
        Title = copy?.Title ?? "";
        Help = copy?.Help ?? "";
        Summary = group is null ? "" : words.Shorthand(group.Key, settings);
        HasHelp = Help.Length > 0;

        Reconcile.Onto(Fields, Rendered(group, words));

        Assert.That(
            IsResolved || Fields.Count == 0,
            "a group the form did not carry draws no fields", Title, Fields.Count);
    }

    /// <summary>
    /// The visible fields, each rendered by the view model that already owns its key. A field
    /// the form marks invisible is left out of the list rather than drawn disabled: it is a
    /// knob whose help text would teach a reader on another selection nothing
    /// (docs/field-availability.md, "The rule").
    /// </summary>
    private IReadOnlyList<FieldViewModel> Rendered(FieldGroup? group, Vocabulary words)
    {
        if (group is null)
        {
            return [];
        }

        var rendered = new List<FieldViewModel>(group.Fields.Count);
        foreach (var field in group.Fields)
        {
            var model = Of(field.Key);
            model.Apply(field, words);

            if (model.IsVisible)
            {
                rendered.Add(model);
            }
        }

        return rendered;
    }

    /// <summary>
    /// The visible control for one field key, or null where the group carries none. Read
    /// through on demand and never cached by the caller, which is what lets a step lay a
    /// field out itself without holding a second copy of what the form said about it.
    ///
    /// Only visible fields answer: a step placing a hidden knob would be drawing a control the
    /// form said not to draw.
    /// </summary>
    public FieldViewModel? Visible(string key)
    {
        Assert.That(key.Length > 0, "a field lookup names the field it looks for");

        foreach (var field in Fields)
        {
            if (field.Key == key)
            {
                return field;
            }
        }

        return null;
    }

    private FieldViewModel Of(string key)
    {
        if (_fields.TryGetValue(key, out var model))
        {
            return model;
        }

        model = new FieldViewModel(key, _write);
        _fields[key] = model;
        return model;
    }
}
