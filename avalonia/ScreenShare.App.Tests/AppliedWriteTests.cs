using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// What happens to a write the form marks applied rather than staged.
///
/// The defect these lock out was a deadlock, and it took the relay with it.
/// Every field of the wizard was staged until a commit, and the only commit is the publish - so the relay's
/// address, which is the one setting the backend reads on a poll of its own, could not reach the backend
/// without a stream being started.
/// The publish that would have carried it was refused, correctly, because the relay it was about to change
/// could not be reached.
/// A reader who moved their relay could type the new address, watch the screen go on saying it could not dial
/// the old one, and have no way out of the app at all.
///
/// The form now says which groups are the settings themselves (<c>form.proto</c>, FieldGroup.applied) and a
/// write to one of those is stored as it is made.
/// Which groups those are is not decided here: these tests state what the shell does with the answer, not
/// what the answer is.
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

    /// <summary>
    /// The write that was impossible before: the relay's address reaches the backend on its own, with nothing
    /// published and nothing else pressed.
    /// </summary>
    [Fact]
    public async Task AWriteToAnAppliedGroupIsStoredWithoutACommit()
    {
        var flow = await FlowAsync();

        Write(flow, "relay.host", "relay.example");

        var stored = Assert.Single(flow.Backend.Saved);
        Assert.Equal("relay.example", stored.Relay.Host);

        // Stored, not started.
        // The two are different effects and only one of them was asked for.
        Assert.Empty(flow.Backend.Started);
    }

    /// <summary>
    /// A staged group keeps its old behaviour, which is the half of this that must not move: a reader
    /// configuring a stream is making a proposal, and a half-configured pipeline becoming what this machine
    /// is on every keystroke is the failure mode in the other direction.
    /// </summary>
    [Fact]
    public async Task AWriteToAStagedGroupIsHeldForTheCommit()
    {
        var flow = await FlowAsync();

        Write(flow, "publish.name", "bjoern");

        Assert.Empty(flow.Backend.Saved);
        Assert.Equal("bjoern", flow.Form.Draft!.Publish.Name);
    }

    /// <summary>
    /// A write the backend refuses says so, in that side's own words, and the next one that lands clears it.
    /// It is a notice and not the banner that blocks the publish: settings that could not be stored are still
    /// settings a stream can be started on.
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
    /// Two writes with the first still unanswered store the newer draft, and store it last.
    ///
    /// Unary calls carry no ordering between them, so writing each one as it arrives would let a burst - a
    /// port spinner held down, a hostname corrected twice - finish out of order and leave an older value
    /// stored than the one on screen.
    /// One write is in flight at a time and what waits behind it is a draft rather than a queue, since they
    /// are all the same settings and the older ones have nothing left to say.
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

        // The second write found one in flight and waited rather than racing it.
        Assert.Equal(1, backend.HeldSaves);
        Assert.Equal("first", Assert.Single(backend.Saved).Relay.Host);

        // Taken before the first is answered, so it is the write that follows it and not the one already
        // sent.
        var follows = backend.NextSaveAsked;
        backend.AnswerSave();
        await follows;

        // And the one that follows carries what the reader last typed.
        Assert.Equal(2, backend.Saved.Count);
        Assert.Equal("second", backend.Saved[1].Relay.Host);
    }
}
