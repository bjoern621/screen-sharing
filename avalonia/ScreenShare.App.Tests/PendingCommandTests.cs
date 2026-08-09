using ScreenShare.App.Mvvm;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The wait every asked-for effect on this surface is drawn from.
///
/// <b>One fact, two readers.</b> A control draws its spinner from the same field the command
/// refuses a second press off, so these tests state what the command was told and assert what
/// both of them would see - never that a flag was assigned.
/// </summary>
public sealed class PendingCommandTests
{
    /// <summary>Runs the dispatched work where it is handed over, which is what a test's UI loop is.</summary>
    private static readonly Action<Action> Inline = action => action();

    /// <summary>
    /// A call that has not been answered is in flight, and it stops being in flight when the
    /// answer lands. Nothing has to remember that it was running.
    /// </summary>
    [Fact]
    public void ACallThatHasNotAnsweredIsInFlightUntilItDoes()
    {
        var held = new TaskCompletionSource();
        var command = new PendingCommand(() => held.Task, Inline);

        Assert.False(command.IsRunning);
        Assert.True(command.CanExecute(null));

        command.Execute(null);

        Assert.True(command.IsRunning);
        Assert.False(command.CanExecute(null));

        Answers.Now(held.SetResult);

        Assert.False(command.IsRunning);
        Assert.True(command.CanExecute(null));
    }

    /// <summary>
    /// A second press while the first call is out asks for nothing. It is the guard that holds
    /// when a press and the round trip race, which is the interval the whole type exists for.
    /// </summary>
    [Fact]
    public void ASecondPressWhileTheFirstCallIsOutAsksForNothing()
    {
        var held = new TaskCompletionSource();
        var asked = 0;

        var command = new PendingCommand(
            () =>
            {
                asked++;
                return held.Task;
            },
            Inline);

        command.Execute(null);
        command.Execute(null);
        command.Execute(null);

        Assert.Equal(1, asked);

        Answers.Now(held.SetResult);
        command.Execute(null);

        Assert.Equal(2, asked);
    }

    /// <summary>
    /// A call that threw past whatever the effect handles still ends the wait. A flag left set
    /// is a control that never comes back, which is worse than the failure that set it.
    /// </summary>
    [Fact]
    public void ACallThatFailedStillEndsTheWait()
    {
        var held = new TaskCompletionSource();
        var command = new PendingCommand(() => held.Task, Inline);

        command.Execute(null);
        Assert.True(command.IsRunning);

        Answers.Now(() => held.SetException(new InvalidOperationException("the transport gave up")));

        Assert.False(command.IsRunning);
        Assert.True(command.CanExecute(null));
    }

    /// <summary>
    /// Both edges are announced, because an owner that renders this state - the setup flow's
    /// publish gate reads it - has to draw the press as well as the answer.
    /// </summary>
    [Fact]
    public void BothEdgesAreAnnouncedToWhoeverRendersThem()
    {
        var held = new TaskCompletionSource();
        var command = new PendingCommand(() => held.Task, Inline);

        var seen = new List<bool>();
        command.Changed += () => seen.Add(command.IsRunning);

        command.Execute(null);
        Answers.Now(held.SetResult);

        Assert.Equal([true, false], seen);
    }

    /// <summary>
    /// Whatever else gates the press is still asked, and is asked on every read rather than
    /// captured - a command made once is pressed on many passes.
    /// </summary>
    [Fact]
    public void WhateverElseGatesThePressIsStillRead()
    {
        var allowed = false;
        var asked = 0;

        var command = new PendingCommand(
            () =>
            {
                asked++;
                return Task.CompletedTask;
            },
            Inline,
            () => allowed);

        Assert.False(command.CanExecute(null));
        command.Execute(null);
        Assert.Equal(0, asked);

        allowed = true;

        Assert.True(command.CanExecute(null));
        command.Execute(null);
        Assert.Equal(1, asked);
    }

    /// <summary>
    /// The completion goes through the loop it was handed, and not through whichever thread the
    /// transport finished on. Everything a command's state is read by is a binding, and those
    /// tolerate one thread.
    /// </summary>
    [Fact]
    public void TheCompletionIsMarshalledThroughTheLoopItWasGiven()
    {
        var held = new TaskCompletionSource();
        var queued = new List<Action>();

        var command = new PendingCommand(() => held.Task, queued.Add);

        command.Execute(null);
        Answers.Now(held.SetResult);

        Assert.True(command.IsRunning);
        Assert.Single(queued);

        queued[0]();

        Assert.False(command.IsRunning);
    }
}
