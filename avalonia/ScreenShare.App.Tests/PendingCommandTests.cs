using ScreenShare.App.Mvvm;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The wait every asked-for effect on this surface is drawn from.
/// One field carries it, read both by the control that draws the spinner and by the refusal of a second press,
/// so what is asserted is what those two readers see rather than that a flag was assigned.
/// </summary>
public sealed class PendingCommandTests
{
    /// <summary>A test's UI loop: the dispatched work runs where it is handed over.</summary>
    private static readonly Action<Action> Inline = action => action();

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

    /// <summary>The interval the type exists for: a press landing while the round trip is out.</summary>
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

    /// <summary>A wait left standing is a control that never comes back.</summary>
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

    /// <summary>The setup flow's publish gate renders this state, so the press is news as much as the answer.</summary>
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

    /// <summary>Asked on each read rather than captured: one command outlives many render passes.</summary>
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

    /// <summary>Bindings are what read this state, and a binding tolerates one thread.</summary>
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
