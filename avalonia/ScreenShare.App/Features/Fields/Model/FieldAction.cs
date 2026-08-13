using ScreenShare.App.Contracts;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Fields.Model;

/// <summary>
/// An effect a screen offers beside one control, rather than a value the control holds.
///
/// There is one today: measuring this machine's upload throughput, offered beside the uplink figure it
/// writes.
/// The form describes no such thing and could not - an effect is a method on the control plane, and which
/// control a shell puts the button next to is placement (<c>docs/ipc-api.md</c>, "The rule").
///
/// <b>The button writes nothing itself.</b> What the measurement finds goes into the draft through the same
/// write every control uses, so a measured figure and a typed one are one value and not two paths into the
/// settings.
///
/// <b>Whether the press is offered is the command's answer, and why it is not is <see cref="Notice"/>.</b> A
/// screen that refuses the effect - a measurement that would compete with a live stream is the case that
/// exists - greys the button through the command's own gate and states the reason beside it, in the place the
/// same screen states why a control is inert (<c>docs/field-availability.md</c>, "A live stream blocks no
/// field").
///
/// A record, so a pass over an unchanged state produces an action that compares equal to the last one and the
/// bound properties are left alone.
/// It is made per pass, because the notice is read from state that moves; <see cref="Command"/> is held by
/// whoever offers the action, which is what makes two passes equal rather than merely alike, and it is the
/// one place the in-flight state lives: it is both what the button spins on and what refuses a second press.
/// </summary>
public sealed record FieldAction
{
    /// <param name="label">What the button says.</param> <param name="tip">What it does and when it is
    /// refused, since the label is one word.</param>
    /// <param name="notice">
    /// What stands about the effect now: why the press is refused where it is, and what the last attempt
    /// answered otherwise.
    /// Empty where there is nothing to say.
    /// </param>
    /// <param name="command">The effect, which holds whether one is already in flight.</param>
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

    /// <summary>
    /// Why the press is refused, or what the last attempt answered.
    /// Empty where the effect is offered and nothing has been said about it.
    /// </summary>
    public string Notice { get; }

    public PendingCommand Command { get; }
}
