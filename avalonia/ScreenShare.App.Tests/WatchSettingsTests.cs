using ScreenShare.Api.V1;
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
/// The watch group is drawn beside the tiles and kept by a save of its own, so watching without publishing
/// needs neither the broadcast setup nor a <c>StartPublish</c> to keep a change.
/// </summary>
public sealed class WatchSettingsTests
{
    private static readonly Action<Action> Inline = action => action();

    /// <summary>A viewer and a wizard on one draft, built the way the window builds them.</summary>
    private sealed record Both(
        ViewerViewModel Viewer, SetupViewModel Setup, FormSession Form, SeededBackend Backend);

    private static async Task<Both> BothAsync(SeededBackend? seed = null)
    {
        var backend = seed ?? new SeededBackend("linux");
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);

        var viewer = new ViewerViewModel(backend, form, session, Inline);
        var setup = new SetupViewModel(backend, form, session, Inline);

        // Stands in for the shell's own render pass: without it a relay snapshot reaches the session and
        // never the roster drawn from it (Features/Shell/ViewModel/ShellViewModel.cs).
        session.Changed += viewer.Apply;

        // Reads every running state once and stops before the reconnect delay, so nothing is left dialling
        // behind the assertions.
        // It is what puts the relay's paths on the roster a decode is opened from.
        _ = session.Start();
        session.Stop();

        await form.Settled;
        return new Both(viewer, setup, form, backend);
    }

    private static FieldViewModel Field(Both both, string key)
        => both.Viewer.Watch.Group.Fields.Single(field => field.Key == key);

    /// <summary>Chooses an option and waits for the form that answers it.</summary>
    private static async Task ChooseAsync(Both both, string key, string value)
    {
        Field(both, key).Options.Single(option => option.Value == value).Choose.Execute(null);
        await both.Form.Settled;
    }

    /// <summary>The group the wizard does not draw, with every field the form marked visible.</summary>
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
    /// One draft behind both screens.
    /// The publish persists the whole settings message, so a draft per screen would overwrite the other's
    /// half.
    /// </summary>
    [Fact]
    public async Task AWriteInTheViewerIsInWhatTheWizardCommits()
    {
        // A backend the commit can be pressed against: the seeded fixture has no reachable relay, and a
        // reachable one is what unlocks the button.
        var backend = new PublishingBackend();
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        var viewer = new ViewerViewModel(backend, form, session, Inline);
        var setup = new SetupViewModel(backend, form, session, Inline);

        // Reads every running state once and stops before the reconnect delay, so nothing is left dialling
        // behind the assertions.
        _ = session.Start();
        session.Stop();
        await form.Settled;

        var chain = viewer.Watch.Group.Fields.Single(field => field.Key == "viewer.render_chain");
        chain.Options.Single(option => option.Value == "sys").Choose.Execute(null);
        await form.Settled;

        setup.Review.StartSharingCommand.Execute(null);

        var started = Assert.Single(backend.Started);
        Assert.Equal("sys", started.Viewer.RenderChain);
    }

    /// <summary>
    /// Saving hands the whole draft over as a settings write and not as a publish, so a reader who never
    /// sends keeps what they chose.
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
    /// A second save asks for a state that already holds, which is a success
    /// (docs/development-principles.md, "Effects across a process boundary").
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
    /// The refusal is the backend's own sentence, and the button stays pressable.
    /// A save that succeeds clears it, which is the render function's usual property applied to a string.
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
    /// The leg a tile receives on is read through the shared draft on every pass, so a leg changed elsewhere
    /// reaches the next decode without the window being reopened.
    /// </summary>
    [Fact]
    public async Task TheTileLegIsReadThroughRatherThanReadOnce()
    {
        var both = await BothAsync();

        await ChooseAsync(both, "viewer.tile_watch_transport", "rtsp");

        Assert.Equal("rtsp", Field(both, "viewer.tile_watch_transport").Readback);
        Assert.Equal("rtsp", both.Form.Draft!.Viewer.TileWatchTransport);
    }

    // --- The leg a decode opens on ------------------------------------------------------

    /// <summary>A backend with one ready path, so the roster has a row to open a tile from.</summary>
    private static SeededBackend Carrying(string path)
    {
        var backend = new SeededBackend("linux");
        var relay = new RelayStatus { Reachable = true };
        relay.Paths.Add(new RelayPath { Name = path, OwnName = path, Ready = true });
        backend.Relay = relay;
        return backend;
    }

    /// <summary>Shows one stream and waits for the decode it opens.</summary>
    private static async Task TileAsync(Both both, string stream)
    {
        var row = both.Viewer.Streams.Single(entry => entry.Name == stream);
        row.Show.Execute(null);
        await both.Form.Settled;
    }

    /// <summary>
    /// The leg is the only knob in the group the shell names in a call: the backend reads the render chain
    /// and both jitter buffers out of its own settings as it builds the pipeline.
    /// Taken off the draft instead, it opens an unkept protocol against kept buffers, and nothing on the
    /// screen says which half of a staged panel took effect.
    /// </summary>
    [Fact]
    public async Task ATileOpensOnTheKeptLegAndNotOnTheDraftedOne()
    {
        var backend = Carrying("desk");
        var both = await BothAsync(backend);

        await ChooseAsync(both, "viewer.tile_watch_transport", "rtsp");
        await TileAsync(both, "desk");

        Assert.Equal("srt", Assert.Single(backend.Decoded).Transport);
    }

    [Fact]
    public async Task AKeptLegIsWhatTheNextTileOpensOn()
    {
        var backend = Carrying("desk");
        var both = await BothAsync(backend);

        await ChooseAsync(both, "viewer.tile_watch_transport", "rtsp");
        both.Viewer.Watch.SaveCommand.Execute(null);
        await TileAsync(both, "desk");

        Assert.Equal("rtsp", Assert.Single(backend.Decoded).Transport);
    }

    /// <summary>
    /// A staged group draws the same controls whether or not it has been kept, so without the notice the
    /// button has nothing to mean.
    /// </summary>
    [Fact]
    public async Task ThePanelSaysWhenWhatItShowsIsNotWhatIsKept()
    {
        var both = await BothAsync();
        Assert.False(both.Viewer.Watch.IsUnkept);

        await ChooseAsync(both, "viewer.render_chain", "sys");
        Assert.True(both.Viewer.Watch.IsUnkept);

        both.Viewer.Watch.SaveCommand.Execute(null);
        Assert.False(both.Viewer.Watch.IsUnkept);
    }

    /// <summary>
    /// The settings travel whole, so an applied field's keystroke stores the watch group with it and the
    /// panel has no difference left to warn about.
    /// </summary>
    [Fact]
    public async Task AnAppliedWriteElsewhereKeepsTheseSettingsToo()
    {
        var both = await BothAsync();

        await ChooseAsync(both, "viewer.render_chain", "sys");
        both.Form.Write("relay.host", new FieldValue { Text = "relay.example" });
        await both.Form.Settled;

        Assert.Equal("sys", Assert.Single(both.Backend.Saved).Viewer.RenderChain);
        Assert.False(both.Viewer.Watch.IsUnkept);
    }

    // --- What a roster row offers -------------------------------------------------------

    /// <summary>
    /// A leg the backend ruled out keeps its place on the menu, greys, and carries the sentence that says
    /// why, which is what <c>docs/field-availability.md</c> states about every unavailable option.
    /// </summary>
    [Fact]
    public async Task ALegTheBackendRuledOutIsGreyedWithItsReason()
    {
        var both = await BothAsync(Carrying("desk"));
        var legs = both.Viewer.Streams.Single(row => row.Name == "desk").Legs;

        var refused = legs.Single(leg => leg.Value == "hls");
        Assert.False(refused.IsEnabled);
        Assert.True(refused.HasReason);
        Assert.Contains("HLS", refused.Reason);

        // The reachable ones are untouched: the greying is a statement, not a screen unsure about everything.
        Assert.All(legs.Where(leg => leg.Value != "hls"), leg =>
        {
            Assert.True(leg.IsEnabled);
            Assert.False(leg.HasReason);
        });
    }

    /// <summary>
    /// Dropping a ruled-out leg would take its sentence with it, and the sentence is what names the legs that
    /// would have worked.
    /// </summary>
    [Fact]
    public async Task ARuledOutLegStaysOnTheMenu()
    {
        var both = await BothAsync(Carrying("desk"));
        var form = await both.Backend.ResolveFormAsync(await both.Backend.SettingsAsync());
        var offered = form.Groups
            .SelectMany(group => group.Fields)
            .Single(field => field.Key == "viewer.player_watch_transport")
            .Options.Select(option => option.Value);

        Assert.Equal(offered, both.Viewer.Streams.Single(row => row.Name == "desk").Legs.Select(leg => leg.Value));
    }

    /// <summary>
    /// The press on an open leg is what closes the player, so greying it would leave one running with the
    /// only control that ends it inert.
    /// </summary>
    [Fact]
    public async Task ARuledOutLegThatIsOpenCanStillBeClosed()
    {
        var backend = Carrying("desk");
        backend.Watching.Add(new WatchKey { StreamName = "desk", Transport = "hls" });
        var both = await BothAsync(backend);

        var open = both.Viewer.Streams.Single(row => row.Name == "desk").Legs.Single(leg => leg.Value == "hls");

        Assert.True(open.IsOpen);
        Assert.True(open.IsEnabled);
    }

    // --- The panel, open and closed -----------------------------------------------------

    /// <summary>
    /// What the reader asked for has happened, and the tiles the panel governs are behind it.
    /// </summary>
    [Fact]
    public async Task KeepingTheSettingsClosesThePanel()
    {
        var both = await BothAsync();
        both.Viewer.ToggleWatchSettings.Execute(null);
        Assert.True(both.Viewer.IsWatchSettingsOpen);

        both.Viewer.Watch.SaveCommand.Execute(null);

        Assert.False(both.Viewer.IsWatchSettingsOpen);
    }

    /// <summary>
    /// The sentence explaining the refusal is on this panel, and dismissing it would take that off the screen
    /// with the fields it is about.
    /// </summary>
    [Fact]
    public async Task ARefusedSaveLeavesThePanelOpen()
    {
        var backend = new SeededBackend("linux") { SaveRefusal = "the settings file could not be written" };
        var both = await BothAsync(backend);
        both.Viewer.ToggleWatchSettings.Execute(null);

        both.Viewer.Watch.SaveCommand.Execute(null);

        Assert.True(both.Viewer.IsWatchSettingsOpen);
        Assert.True(both.Viewer.Watch.HasNotice);
    }

    /// <summary>
    /// A reader who moved a control and then dismissed the panel decided against the change, so nothing is
    /// stored on the way out.
    /// </summary>
    [Fact]
    public async Task ClosingThePanelKeepsNothing()
    {
        var both = await BothAsync();
        both.Viewer.ToggleWatchSettings.Execute(null);

        await ChooseAsync(both, "viewer.render_chain", "sys");
        both.Viewer.Watch.CloseCommand.Execute(null);

        Assert.False(both.Viewer.IsWatchSettingsOpen);
        Assert.Empty(both.Backend.Saved);
        Assert.True(both.Viewer.Watch.IsUnkept);
    }

    /// <summary>
    /// Closing a closed panel changes nothing, which is what lets a commit run it without asking whether it
    /// has to.
    /// </summary>
    [Fact]
    public async Task ClosingAClosedPanelIsNotAnOpen()
    {
        var both = await BothAsync();
        Assert.False(both.Viewer.IsWatchSettingsOpen);

        both.Viewer.Watch.CloseCommand.Execute(null);

        Assert.False(both.Viewer.IsWatchSettingsOpen);
    }
}
