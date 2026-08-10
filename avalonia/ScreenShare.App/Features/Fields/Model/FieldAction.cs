using ScreenShare.App.Contracts;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Fields.Model;

/// <summary>
/// An effect a screen offers beside one control, rather than a value the control holds.
///
/// There is one today: measuring this machine's upload throughput, offered beside the uplink
/// figure it writes. The form describes no such thing and could not - an effect is a method on
/// the control plane, and which control a shell puts the button next to is placement
/// (<c>docs/ipc-api.md</c>, "The rule").
///
/// <b>The button writes nothing itself.</b> What the measurement finds goes into the draft
/// through the same write every control uses, so a measured figure and a typed one are one
/// value and not two paths into the settings.
///
/// A record, so a render pass over an unchanged field hands the same action back and the
/// bound properties are left alone. <see cref="Command"/> is held rather than made per pass,
/// which is what makes two passes equal rather than merely alike, and it is the one place the
/// in-flight state lives: it is both what the button spins on and what refuses a second press.
/// </summary>
public sealed record FieldAction
{
    /// <param name="label">What the button says.</param>
    /// <param name="tip">What it does and when it is refused, since the label is one word.</param>
    /// <param name="command">The effect, which holds whether one is already in flight.</param>
    public FieldAction(string label, string tip, PendingCommand command)
    {
        Assert.That(label.Length > 0, "an action beside a control says what it does");
        Assert.That(tip.Length > 0, "an action beside a control explains itself");
        Assert.NotNull(command, "an action beside a control runs something");

        Label = label;
        Tip = tip;
        Command = command;
    }

    public string Label { get; }

    public string Tip { get; }

    public PendingCommand Command { get; }
}
