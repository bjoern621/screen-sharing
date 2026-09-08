using System.Windows.Input;
using Avalonia;
using Avalonia.Controls.Primitives;
using ScreenShare.App.Contracts;
using TablerIcons;

namespace ScreenShare.App.Controls;

/// <summary>
/// How far one pre-publish line has got.
/// Note, Warned and Blocking are the contract's three severities, and the hue follows: red on Blocking,
/// amber on Warned, none on Note.
/// </summary>
public enum CheckState
{
    Passed,

    /// <summary>Not answerable yet. Blocks nothing.</summary>
    Pending,

    /// <summary><c>SEVERITY_INFO</c>. Stream works.</summary>
    Note,

    /// <summary><c>SEVERITY_WARNING</c>. Stream runs, something about it disappoints.</summary>
    Warned,

    /// <summary>Sharing waits on it. <see cref="CheckItem.FixedInStep"/> names where it is fixed.</summary>
    Blocking,
}

/// <summary>
/// One line of the preflight list.
/// Same list on every setup step and on the review, so a fault shows while a step can still fix it rather
/// than at the end.
/// </summary>
public sealed class CheckItem : TemplatedControl
{
    public static readonly StyledProperty<string> TextProperty =
        AvaloniaProperty.Register<CheckItem, string>(nameof(Text), "");

    public static readonly StyledProperty<CheckState> StateProperty =
        AvaloniaProperty.Register<CheckItem, CheckState>(nameof(State), CheckState.Pending);

    /// <summary>Press reaching the step that fixes the line, empty where the form named no field.</summary>
    public static readonly StyledProperty<string> FixedInStepProperty =
        AvaloniaProperty.Register<CheckItem, string>(nameof(FixedInStep), "");

    /// <summary>What that press runs. Null draws the step as a hint rather than a control.</summary>
    public static readonly StyledProperty<ICommand?> FixProperty =
        AvaloniaProperty.Register<CheckItem, ICommand?>(nameof(Fix));

    public string Text
    {
        get => GetValue(TextProperty);
        set => SetValue(TextProperty, value);
    }

    public CheckState State
    {
        get => GetValue(StateProperty);
        set => SetValue(StateProperty, value);
    }

    public string FixedInStep
    {
        get => GetValue(FixedInStepProperty);
        set => SetValue(FixedInStepProperty, value);
    }

    public ICommand? Fix
    {
        get => GetValue(FixProperty);
        set => SetValue(FixProperty, value);
    }

    /// <summary>
    /// Glyph for a state, as a Tabler icon rather than a text character: a tick off the platform text face lands
    /// at a different weight from the icons beside it.
    /// Exhaustive, so a state added to the enum fails here.
    /// </summary>
    public static Icons GlyphOf(CheckState state) => state switch
    {
        CheckState.Passed => Icons.IconCheck,
        CheckState.Pending => Icons.IconDots,
        CheckState.Note => Icons.IconInfoCircle,
        CheckState.Warned => Icons.IconAlertTriangle,
        CheckState.Blocking => Icons.IconExclamationMark,
        _ => Assert.Never<Icons>("unexpected check state", (int)state),
    };
}
