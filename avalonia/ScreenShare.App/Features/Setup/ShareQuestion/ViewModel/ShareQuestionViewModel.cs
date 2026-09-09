using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.RegionPicker.ViewModel;
using ScreenShare.App.Features.Setup.ScreenPicker.ViewModel;
using ScreenShare.App.Features.Setup.WindowPicker.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.ShareQuestion.ViewModel;

/// <summary>
/// What the stream shares, as one question: the kind first, then the one chooser it names
/// (<see cref="ShareLayout"/>).
///
/// <b>Three choosers in place of a dropdown and a text box.</b>
/// A screen is picked off a live picture of it, a window off its title, and a rectangle by dragging one.
/// Each is drawn from the control it writes, so the backend hiding the two controls the kind does not name
/// leaves one chooser on screen with no gate here.
/// Anything else the group grows keeps its generic row (<see cref="Rest"/>).
///
/// <b>Asked at the press that starts a stream.</b>
/// What is shared is somebody's answer per stream rather than a machine's settled one,
/// so it is drawn in the dialog that press puts up and on no wizard step
/// (<see cref="Fields.Model.GroupPlacement"/>).
///
/// <b>Outputs</b> past those inputs. The answer leaves through the draft:
/// the choosers write where every other control writes, and the press that starts a stream reads it back.
/// </summary>
public sealed class ShareQuestionViewModel : Observable
{
    private readonly FormSession _form;
    private readonly Session _session;

    /// <summary>View drawing this question is on screen with the window in front. Written by that view.</summary>
    private bool _showing;

    /// <summary>Dialog over the window is up. Written by the picker.</summary>
    private bool _inDialog;

    /// <summary>
    /// Pass in flight, and whether one was asked for inside it.
    /// A window read landing inside a pass asks for another rather than nesting one in it.
    /// </summary>
    private bool _rendering;
    private bool _again;

    public ShareQuestionViewModel(
        IBackend backend, FormSession form, Session session, Func<Task<string>> drawRegion, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "the share question reads what this machine has to offer");
        Assert.NotNull(form, "the share question draws the draft the window is holding");
        Assert.NotNull(session, "the share question names its entries out of what the backend answered");
        Assert.NotNull(drawRegion, "the share question needs somewhere to draw a rectangle");
        Assert.NotNull(dispatch, "the share question marshals a read back to the UI loop");

        _form = form;
        _session = session;

        Group = new FieldGroupViewModel(_form.Write);

        Screens = new ScreenPickerViewModel(
            backend, session, dispatch,
            monitor => _form.Write(ShareLayout.MonitorKey, new FieldValue { Number = monitor }));

        Windows = new WindowPickerViewModel(
            backend, dispatch,
            handle => _form.Write(ShareLayout.WindowKey, new FieldValue { Text = handle }));

        Region = new RegionPickerViewModel(
            drawRegion, dispatch,
            region => _form.Write(ShareLayout.RegionKey, new FieldValue { Text = region }));

        Windows.Changed += Apply;
        _form.Changed += Apply;

        Apply();
    }

    // --- Outputs ------------------------------------------------------------------

    private FieldViewModel? _kind;

    /// <summary>The group as the generic renderer sees it, which the three choosers read their fields out of.</summary>
    public FieldGroupViewModel Group { get; }

    /// <summary>One live picture per screen, under the kind that names a screen.</summary>
    public ScreenPickerViewModel Screens { get; }

    /// <summary>One row per open window, under the kind that names a window.</summary>
    public WindowPickerViewModel Windows { get; }

    /// <summary>The drawn rectangle and the press that draws another, under the kind that names one.</summary>
    public RegionPickerViewModel Region { get; }

    /// <summary>
    /// The kind, drawn above the choosers.
    /// Null where the form carries no such control, which is a backend whose desktop picks for itself.
    /// </summary>
    public FieldViewModel? Kind { get => _kind; private set => Set(ref _kind, value); }

    /// <summary>
    /// Every other control the group carries, as the generic rows under the chooser.
    /// A control the backend adds to the group appears here with nothing on this side to edit.
    /// </summary>
    public ObservableCollection<FieldViewModel> Rest { get; } = [];

    /// <summary>
    /// Whether this machine has anything to ask before a stream starts.
    /// False where the share group carries no control the reader can move:
    /// the desktop's own picker answers there, and a question offering nothing would stand between
    /// the press and the stream for no reason.
    /// </summary>
    public bool Asks
    {
        get
        {
            var group = GroupOf(_form.Form);
            if (group is null)
            {
                return false;
            }

            foreach (var control in group.Fields)
            {
                if (control.Visible && control.Enabled)
                {
                    return true;
                }
            }

            return false;
        }
    }

    // --- Inputs -------------------------------------------------------------------

    /// <summary>Named write of whether the dialog over the window is up.</summary>
    public void SetInDialog(bool inDialog)
    {
        _inDialog = inDialog;
        Apply();
    }

    /// <summary>
    /// Named write of whether the view of this question is being looked at, idempotent.
    /// Called by that view, tree membership and window activation being visible to the control
    /// and the platform alone.
    /// </summary>
    public void SetShowing(bool showing)
    {
        _showing = showing;
        Screens.SetShowing(_showing);
        Apply();
    }

    // --- Lifecycle ------------------------------------------------------------------

    /// <summary>
    /// The one render function.
    /// Safe to run twice: the group's own pass produces fields that compare equal,
    /// and an unmoved draft asks for no resolve.
    /// A read landing inside a pass asks for another rather than nesting one in it.
    /// </summary>
    public void Apply()
    {
        if (_rendering)
        {
            _again = true;
            return;
        }

        _rendering = true;
        try
        {
            do
            {
                _again = false;
                Render();
            }
            while (_again);
        }
        finally
        {
            _rendering = false;
        }
    }

    private void Render()
    {
        _form.Sync();

        var form = _form.Form;
        var group = GroupOf(form);

        // The windows the last read found name the handles the form offers,
        // so the words carry them wherever an entry of that control is drawn.
        var words = _session.Words.With(Windows.Listed);
        Group.Apply(group, words, form?.Settings, _form.IsAnswered);

        // The choosers first, the rows below being what none of them drew.
        Screens.Apply(FieldOf(group, ShareLayout.MonitorKey), _inDialog);
        Windows.Apply(FieldOf(group, ShareLayout.WindowKey), words, _inDialog);
        Region.Apply(Group.Visible(ShareLayout.RegionKey));

        Kind = Group.Visible(ShareLayout.KindKey);
        Reconcile.Onto(Rest, [.. Group.Fields.Where(Generic)]);

        Assert.That(
            Kind is null || Kind.Key == ShareLayout.KindKey, "the kind control is the one the layout names", Kind?.Key);
    }

    /// <summary>Whether a control takes the generic row, which is every one no chooser drew.</summary>
    private bool Generic(FieldViewModel field)
        => field.Key != ShareLayout.KindKey && !Chosen(field.Key);

    /// <summary>
    /// Whether a chooser of this step's own is drawing that control.
    /// Read off the chooser rather than off the key,
    /// so a control the chooser cannot draw keeps its plain row and stays reachable:
    /// a session that cannot read one screen apart from another gets the list back.
    /// </summary>
    private bool Chosen(string key) => key switch
    {
        ShareLayout.MonitorKey => Screens.IsVisible,
        ShareLayout.WindowKey => Windows.IsVisible,
        ShareLayout.RegionKey => Region.IsVisible,
        _ => false,
    };

    private static FieldGroup? GroupOf(Form? form)
    {
        if (form is null)
        {
            return null;
        }

        foreach (var group in form.Groups)
        {
            if (group.Key == ShareLayout.GroupKey)
            {
                return group;
            }
        }

        return null;
    }

    private static Field? FieldOf(FieldGroup? group, string key)
    {
        if (group is null)
        {
            return null;
        }

        foreach (var field in group.Fields)
        {
            if (field.Key == key)
            {
                return field;
            }
        }

        return null;
    }
}
