using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Where a write lands when the form marks its group applied rather than staged.
///
/// Deadlock this locks out: staging every field until a commit, where the only commit is the publish,
/// gets the relay address to the backend only once a stream is started,
/// and that address is the one setting the backend reads on a poll of its own.
/// The publish carrying it is refused, correctly, the relay it is about to change being undiallable.
///
/// Which groups are the settings themselves is the form's answer (<c>form.proto</c>, FieldGroup.applied).
/// Asserted here is what the shell does with that answer, not what the answer is.
/// </summary>
public sealed class AppliedWriteTests
{
    private static readonly Action<Action> Inline = action => action();

    private sealed record Flow(SetupViewModel Setup, FormSession Form, SeededBackend Backend);

    private static async Task<Flow> FlowAsync()
    {
        var backend = new SeededBackend("linux");
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        var setup = new SetupViewModel(backend, form, session, Inline);

        await form.Settled;
        return new Flow(setup, form, backend);
    }

    private static void Write(Flow flow, string key, string value)
        => flow.Form.Write(key, new FieldValue { Text = value });

    /// <summary>Relay address reaches the backend with nothing published and nothing else pressed.</summary>
    [Fact]
    public async Task AWriteToAnAppliedGroupIsStoredWithoutACommit()
    {
        var flow = await FlowAsync();

        Write(flow, "relay.host", "relay.example");

        var stored = Assert.Single(flow.Backend.Saved);
        Assert.Equal("relay.example", stored.Relay.Host);

        // Storing and starting are different effects, and only one was asked for.
        Assert.Empty(flow.Backend.Started);
    }

    /// <summary>
    /// Half that must not move.
    /// A reader configuring a stream is making a proposal,
    /// and a half-configured pipeline becoming what this machine runs on every keystroke is the other failure.
    /// </summary>
    [Fact]
    public async Task AWriteToAStagedGroupIsHeldForTheCommit()
    {
        var flow = await FlowAsync();

        flow.Form.Write("publish.fps", new FieldValue { Number = 120 });

        Assert.Empty(flow.Backend.Saved);
        Assert.Equal(120, flow.Form.Draft!.Publish.Fps);
    }

    /// <summary>
    /// A notice rather than the banner blocking the publish:
    /// settings that could not be stored are still settings a stream can start on.
    /// Reason is the backend's own words, and the next write that lands clears it.
    /// </summary>
    [Fact]
    public async Task AWriteThatCouldNotBeStoredNamesTheReason()
    {
        var flow = await FlowAsync();
        flow.Backend.SaveRefusal = "the settings file could not be written";

        Write(flow, "relay.host", "relay.example");
        flow.Setup.Apply();

        Assert.Equal("the settings file could not be written", flow.Form.Unsaved);
        Assert.True(flow.Setup.HasUnsaved);
        Assert.Equal("the settings file could not be written", flow.Setup.Unsaved);
        Assert.False(flow.Setup.IsUnavailable);

        flow.Backend.SaveRefusal = "";
        Write(flow, "relay.host", "relay.example.two");
        flow.Setup.Apply();

        Assert.Equal("", flow.Form.Unsaved);
        Assert.False(flow.Setup.HasUnsaved);
    }

    /// <summary>
    /// Unary calls carry no ordering between them,
    /// so writing each one as it arrives lets a burst finish out of order and store an older value than the screen's.
    /// One write is in flight at a time, and what waits behind it is a draft rather than a queue:
    /// they are all the same settings, and the older ones have nothing left to say.
    /// </summary>
    [Fact]
    public async Task TwoWritesInFlightStoreTheNewestDraftLast()
    {
        var backend = new DeferredBackend { DefersSaves = true };
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        await backend.AnswerAsync(0);

        form.Write("relay.host", new FieldValue { Text = "first" });
        form.Write("relay.host", new FieldValue { Text = "second" });

        Assert.Equal(1, backend.HeldSaves);
        Assert.Equal("first", Assert.Single(backend.Saved).Relay.Host);

        // Taken before the first save is answered, so it awaits the write that follows rather than the one sent.
        var follows = backend.NextSaveAsked;
        backend.AnswerSave();
        await follows;

        Assert.Equal(2, backend.Saved.Count);
        Assert.Equal("second", backend.Saved[1].Relay.Host);
    }
}
