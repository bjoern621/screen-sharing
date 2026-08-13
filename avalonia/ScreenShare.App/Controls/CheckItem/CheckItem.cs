using Avalonia;
using Avalonia.Controls.Primitives;
using ScreenShare.App.Contracts;
using TablerIcons;

namespace ScreenShare.App.Controls;

/// <summary>
/// How far one pre-publish line has got.
/// Five states rather than a bool, and the middle three are the contract's three severities: a note worth
/// knowing, something likely to disappoint, and something that stops the publish are three different things
/// to a reader, and only the last of them may spend the palette's one hue.
/// </summary>
public enum CheckState
{
    /// <summary>Satisfied. The reader watches these turn over as they work.</summary>
    Passed,

    /// <summary>Not answerable yet, and not blocking anything.</summary>
    Pending,

    /// <summary>Worth knowing; the stream works. <c>SEVERITY_INFO</c>.</summary>
    Note,

    /// <summary>The stream runs and something about it is likely to disappoint. <c>SEVERITY_WARNING</c>.</summary>
    Warned,

    /// <summary>Failing, and sharing waits on it. <see cref="CheckItem.FixedInStep"/> says where.</summary>
    Blocking,
}

/// <summary>
/// One line of the preflight list.
/// The list is the same on every setup step and on the review, so the reader meets it early and watches it
/// resolve rather than hitting a validation wall at the end.
///
/// A blocking item names the step that fixes it, which is the whole reason nobody has to hunt for the control
/// at fault.
/// </summary>
public sealed class CheckItem : TemplatedControl
{
    /// <summary>What is being checked, in prose.</summary>
    public static readonly StyledProperty<string> TextProperty =
        AvaloniaProperty.Register<CheckItem, string>(nameof(Text), "");

    public static readonly StyledProperty<CheckState> StateProperty =
        AvaloniaProperty.Register<CheckItem, CheckState>(nameof(State), CheckState.Pending);

    /// <summary>
    /// The step that fixes a blocking check, empty on the others.
    /// A view shows it only where it applies, so a passed item never carries a dangling "fix in" clause.
    /// </summary>
    public static readonly StyledProperty<string> FixedInStepProperty =
        AvaloniaProperty.Register<CheckItem, string>(nameof(FixedInStep), "");

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

    /// <summary>
    /// The glyph the state wears, as a Tabler icon rather than a character: a tick, an ellipsis and an
    /// exclamation drawn by the platform text face would each be a different weight from the icons beside
    /// them.
    /// Exhaustive, so a fourth state fails here first.
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
