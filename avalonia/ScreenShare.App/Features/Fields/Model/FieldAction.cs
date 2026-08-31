using ScreenShare.App.Contracts;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Fields.Model;

/// <summary>
/// Effect a screen offers beside one control, as against a value the control holds.
/// Measuring this machine's upload throughput, offered beside the uplink figure it writes, is the shape.
///
/// The form describes none of it: an effect is a method on the control plane,
/// and which control a shell puts the button beside is placement (<c>docs/ipc-api.md</c>, "The rule").
///
/// The button writes nothing itself.
/// What the measurement finds reaches the draft through the write every control uses, so a measured figure
/// and a typed one are one path into the settings.
///
/// Whether the press is offered is <see cref="Command"/>'s own answer, and why it is not is <see cref="Notice"/>,
/// stated where a greyed control's reason is stated (<c>docs/field-availability.md</c>,
/// "A live stream blocks no field").
///
/// A record, so a pass over unchanged state produces an action that compares equal to the last
/// and the bound properties are left alone.
/// Built per pass, since the notice is read from state that moves.
/// <see cref="Command"/> is held by whoever offers the action, so two passes compare equal rather than merely alike,
/// and it is the one place the in-flight state lives: what the button spins on, and what refuses a second press.
/// </summary>
public sealed record FieldAction
{
    /// <param name="label">What the button says.</param>
    /// <param name="tip">What the effect does and when it is refused, since the label is one word.</param>
    /// <param name="notice">Why the press is refused, or what the last attempt answered.</param>
    /// <param name="command">Effect, holding whether one is already in flight.</param>
    public FieldAction(string label, string tip, string notice, PendingCommand command)
    {
        Assert.That(label.Length > 0, "an action beside a control says what it does");
        Assert.That(tip.Length > 0, "an action beside a control explains itself");
        Assert.NotNull(notice, "an action beside a control carries a sentence or the empty one");
        Assert.NotNull(command, "an action beside a control runs something");

        Label = label;
        Tip = tip;
        Notice = notice;
        Command = command;
    }

    public string Label { get; }

    public string Tip { get; }

    /// <summary>Empty where the effect is offered and nothing has been said about it.</summary>
    public string Notice { get; }

    public PendingCommand Command { get; }
}
