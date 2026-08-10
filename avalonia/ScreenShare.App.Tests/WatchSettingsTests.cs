using ScreenShare.App.Backend;
using ScreenShare.App.Features.Fields.Model;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.ViewModel;
using ScreenShare.App.Features.Viewer.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Where the settings about receiving live, and what they do.
///
/// Two defects are locked out here, and they were one misplacement. The watch group was a step
/// of the sending wizard, so a reader watching without publishing had to open the broadcast
/// setup to change how their tiles decode - and once there, the only thing that persisted the
/// change was going live, because the wizard's draft reaches the backend through
/// <c>StartPublish</c>. The group is drawn beside the tiles now and has a save of its own.
/// </summary>
public sealed class WatchSettingsTests
{
    private static readonly Action<Action> Inline = action => action();

    /// <summary>
    /// A viewer and a wizard on one draft, built the way the window builds them. The pair is what
    /// half of these tests are about, so the fixture makes it rather than each test.
    /// </summary>
    private sealed record Both(
        ViewerViewModel Viewer, SetupViewModel Setup, FormSession Form, SeededBackend Backend);

    private static async Task<Both> BothAsync(SeededBackend? seed = null)
    {
        var backend = seed ?? new SeededBackend("linux");
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);

        var viewer = new ViewerViewModel(backend, form, session, Inline);
        var setup = new SetupViewModel(backend, form, session, Inline);

        await form.Settled;
        return new Both(viewer, setup, form, backend);
    }

    private static FieldViewModel Field(Both both, string key)
        => both.Viewer.Watch.Group.Fields.Single(field => field.Key == key);

    /// <summary>Picks one entry and waits for the form that answers the pick.</summary>
    private static async Task ChooseAsync(Both both, string key, string value)
    {
        Field(both, key).Options.Single(option => option.Value == value).Choose.Execute(null);
        await both.Form.Settled;
    }

    /// <summary>The viewer draws the group the wizard does not, and every control the form put in it.</summary>
    [Fact]
    public async Task TheViewerDrawsTheWatchingGroup()
    {
        var both = await BothAsync();
        var group = (await both.Backend.ResolveFormAsync(await both.Backend.SettingsAsync()))
            .Groups.Single(g => GroupPlacement.InViewer(g.Key));

        Assert.True(both.Viewer.Watch.Group.IsResolved);
        Assert.Equal(
            group.Fields.Where(field => field.Visible).Select(field => field.Key),
            both.Viewer.Watch.Group.Fields.Select(field => field.Key));
    }

    /// <summary>
    /// One draft behind both screens. A write in the viewer is in the message the wizard commits,
    /// which is the property a draft per screen could not have: the publish persists the whole
    /// settings message, so the two would overwrite each other's half.
    /// </summary>
    [Fact]
    public async Task AWriteInTheViewerIsInWhatTheWizardCommits()
    {
        // A backend the commit can actually be pressed against: the seeded fixture has no relay
        // behind it, and a reachable one is what unlocks the button.
        var backend = new PublishingBackend();
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        var viewer = new ViewerViewModel(backend, form, session, Inline);
        var setup = new SetupViewModel(backend, form, session, Inline);

        // Reads every running state once and stops before the reconnect delay, so what the
        // session found is on it and nothing is left dialling behind the assertions.
        _ = session.Start();
        session.Stop();
        await form.Settled;

        var chain = viewer.Watch.Group.Fields.Single(field => field.Key == "viewer.render_chain");
        chain.Options.Single(option => option.Value == "sys").Choose.Execute(null);
        await form.Settled;

        setup.Review.GoLiveCommand.Execute(null);

        var started = Assert.Single(backend.Started);
        Assert.Equal("sys", started.Viewer.RenderChain);
    }

    /// <summary>
    /// Saving hands the whole draft over. It is a settings write and not a publish, so a reader
    /// who never sends still keeps what they chose.
    /// </summary>
    [Fact]
    public async Task SavingKeepsTheDraftWithoutPublishing()
    {
        var both = await BothAsync();

        await ChooseAsync(both, "viewer.render_chain", "sys");
        both.Viewer.Watch.SaveCommand.Execute(null);

        var saved = Assert.Single(both.Backend.Saved);
        Assert.Equal("sys", saved.Viewer.RenderChain);
        Assert.Empty(both.Backend.Started);
        Assert.False(both.Viewer.Watch.HasNotice);
    }

    /// <summary>
    /// Saving twice with nothing changed asks for a state that already holds, which is a success
    /// and not a refusal (docs/development-principles.md, "Effects across a process boundary").
    /// </summary>
    [Fact]
    public async Task SavingTwiceIsNotAnError()
    {
        var both = await BothAsync();

        both.Viewer.Watch.SaveCommand.Execute(null);
        both.Viewer.Watch.SaveCommand.Execute(null);

        Assert.Equal(2, both.Backend.Saved.Count);
        Assert.False(both.Viewer.Watch.HasNotice);
        Assert.True(both.Viewer.Watch.SaveCommand.CanExecute(null));
    }

    /// <summary>
    /// A save the backend refuses shows that side's own sentence and leaves the button pressable.
    /// The next save that succeeds clears it, which is the render function's usual property
    /// applied to a string.
    /// </summary>
    [Fact]
    public async Task ARefusedSaveShowsTheBackendsSentenceAndClearsOnTheNextOne()
    {
        var backend = new SeededBackend("linux") { SaveRefusal = "the settings file could not be written" };
        var both = await BothAsync(backend);

        both.Viewer.Watch.SaveCommand.Execute(null);

        Assert.True(both.Viewer.Watch.HasNotice);
        Assert.Equal("the settings file could not be written", both.Viewer.Watch.Notice);
        Assert.True(both.Viewer.Watch.SaveCommand.CanExecute(null));

        backend.SaveRefusal = "";
        both.Viewer.Watch.SaveCommand.Execute(null);

        Assert.False(both.Viewer.Watch.HasNotice);
    }

    /// <summary>
    /// The leg a tile receives on is read through the shared draft on every pass. It used to be
    /// read once when the screen mounted, so a leg changed anywhere else did not reach a decode
    /// until the window was reopened.
    /// </summary>
    [Fact]
    public async Task TheTileLegIsReadThroughRatherThanReadOnce()
    {
        var both = await BothAsync();

        await ChooseAsync(both, "viewer.tile_watch_transport", "rtsp");

        Assert.Equal("rtsp", Field(both, "viewer.tile_watch_transport").Readback);
        Assert.Equal("rtsp", both.Form.Draft!.Viewer.TileWatchTransport);
    }
}
