using ScreenShare.App.Contracts;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Fields.Model;

/// <summary>
/// An effect a screen offers beside a group's heading, as against beside one of its controls
/// (<see cref="FieldAction"/>).
/// It sits on the heading because it is about every setting under it, and a field carrying it would speak for
/// its neighbours.
/// Putting a group back to what a fresh installation holds is the shape.
///
/// Carries no values.
/// What each field starts as is stated per field by the form (<c>form.proto</c>, <c>Field.default_value</c>), so
/// the press reaches the draft through the writes every control uses and this side holds no table of defaults.
///
/// A record, so a pass over unchanged state produces an action that compares equal to the last and the bound
/// properties are left alone.
/// <see cref="Command"/> is held by whoever offers the action, so two passes compare equal rather than merely
/// alike.
/// </summary>
public sealed record GroupAction
{
    /// <param name="label">What the button says.</param>
    /// <param name="tip">What it changes and where the values come from, since the label is one word.</param>
    /// <param name="command">Effect.</param>
    public GroupAction(string label, string tip, DelegateCommand command)
    {
        Assert.That(label.Length > 0, "an action beside a heading says what it does");
        Assert.That(tip.Length > 0, "an action beside a heading explains itself");
        Assert.NotNull(command, "an action beside a heading runs something");

        Label = label;
        Tip = tip;
        Command = command;
    }

    public string Label { get; }

    public string Tip { get; }

    public DelegateCommand Command { get; }
}
