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
/// Two defects are locked out here, and they were one misplacement.
/// The watch group was a step of the sending wizard, so a reader watching without publishing had to open the
/// broadcast setup to change how their tiles decode - and once there, the only thing that persisted the
/// change was starting to share, because the wizard's draft reaches the backend through <c>StartPublish</c>.
/// The group is drawn beside the tiles now and has a save of its own.
/// </summary>
public sealed class WatchSettingsTests
{
    private static readonly Action<Action> Inline = action => action();

    /// <summary>
    /// A viewer and a wizard on one draft, built the way the window builds them.
    /// The pair is what half of these tests are about, so the fixture makes it rather than each test.
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

        // The shell renders its destinations when the running state announces, so the fixture stands in for
        // that pass: without it a relay snapshot would reach the session and never the roster drawn from it
        // (Features/Shell/ViewModel/ShellViewModel.cs).
        session.Changed += viewer.Apply;

        // Reads every running state once and stops before the reconnect delay, so what the session found is
        // on it and nothing is left dialling behind the assertions.
        // It is what puts the relay's paths on the roster, which the tests about opening a decode need.
        _ = session.Start();
        session.Stop();

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
    /// One draft behind both screens.
    /// A write in the viewer is in the message the wizard commits, which is the property a draft per screen
    /// could not have: the publish persists the whole settings message, so the two would overwrite each
    /// other's half.
    /// </summary>
    [Fact]
    public async Task AWriteInTheViewerIsInWhatTheWizardCommits()
    {
        // A backend the commit can actually be pressed against: the seeded fixture has no relay behind it,
        // and a reachable one is what unlocks the button.
        var backend = new PublishingBackend();
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        var viewer = new ViewerViewModel(backend, form, session, Inline);
        var setup = new SetupViewModel(backend, form, session, Inline);

        // Reads every running state once and stops before the reconnect delay, so what the session found is
        // on it and nothing is left dialling behind the assertions.
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
    /// Saving hands the whole draft over.
    /// It is a settings write and not a publish, so a reader who never sends still keeps what they chose.
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
    /// Saving twice with nothing changed asks for a state that already holds, which is a success and not a
    /// refusal (docs/development-principles.md, "Effects across a process boundary").
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
    /// The next save that succeeds clears it, which is the render function's usual property applied to a
    /// string.
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
    /// The leg a tile receives on is read through the shared draft on every pass.
    /// It used to be read once when the screen mounted, so a leg changed anywhere else did not reach a decode
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

    // --- What a decode opens on ---------------------------------------------------------

    /// <summary>A backend carrying one ready path, which is what gives the roster a row to tile.</summary>
    private static SeededBackend Carrying(string path)
    {
        var backend = new SeededBackend("linux");
        var relay = new RelayStatus { Reachable = true };
        relay.Paths.Add(new RelayPath { Name = path, Ready = true });
        backend.Relay = relay;
        return backend;
    }

    /// <summary>Puts one stream in the grid and waits for the decode that answers.</summary>
    private static async Task TileAsync(Both both, string stream)
    {
        var row = both.Viewer.Streams.Single(entry => entry.Name == stream);
        row.Show.Execute(null);
        await both.Form.Settled;
    }

    /// <summary>
    /// A decode opens on the leg that was kept, not on the one the panel is showing.
    ///
    /// <b>This is the defect the panel's button exists against.</b> The leg is the only one of the group's
    /// six knobs the shell names in a call - the backend reads the render chain and both jitter buffers out
    /// of its own settings when it builds the pipeline - so a leg taken from the draft opened an unkept
    /// protocol against kept buffers.
    /// Half a staged panel taking effect as it was edited is worse than none of it doing so, because nothing
    /// on the screen says which half.
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

    /// <summary>And the kept leg is what the next decode opens on, which is what keeping means.</summary>
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
    /// The panel says when what it shows is not what a decode would open on, and stops saying it once the two
    /// agree.
    /// Without it a staged group looks identical either way, which leaves the button nothing to mean.
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
    /// A write from the wizard does not leave the viewer's panel claiming to be unkept.
    /// Both screens edit one draft and the settings travel whole, so an applied field's keystroke stores the
    /// watch group with it - and a panel that went on warning would be describing a difference that is not
    /// there.
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

    // --- What the roster offers ---------------------------------------------------------

    /// <summary>
    /// A leg the backend ruled out keeps its place on the menu, greys, and carries the sentence that says
    /// why.
    /// It is the one rule <c>docs/field-availability.md</c> states about an unavailable option, and this menu
    /// used to be the one control in the product that did not keep it: the roster took the option's value and
    /// label and threw the verdict away, so a protocol no player here opens was offered as though it were
    /// live and answered with a refusal only after the press.
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

        // And the reachable ones are untouched, which is what makes the greying a statement rather than a
        // screen that greys everything it is unsure about.
        Assert.All(legs.Where(leg => leg.Value != "hls"), leg =>
        {
            Assert.True(leg.IsEnabled);
            Assert.False(leg.HasReason);
        });
    }

    /// <summary>
    /// A ruled-out leg is greyed and not dropped.
    /// Removing it would take the sentence with it, and the sentence is what names the legs that would have
    /// worked.
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
    /// A leg already open stays pressable however the availability pass answers for it.
    /// The press closes the player, and greying it would leave one running with the only control that ends it
    /// inert - a worse state than the one the greying is about.
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

    // --- Opening and closing the panel --------------------------------------------------

    /// <summary>
    /// Keeping the settings shuts the panel.
    /// The reader asked for one thing and it happened, so there is nothing left on the column to look at -
    /// and the tiles it governs are behind it.
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
    /// A refused save leaves it open.
    /// The sentence explaining the refusal is on this panel, and dismissing it would take that off the screen
    /// along with the fields it is about.
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
    /// The panel's own button shuts it and keeps nothing.
    /// A reader who moved a control and then dismissed the panel decided against the change, and storing it
    /// on the way out would be the screen keeping what nobody asked it to.
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
    /// Closing a panel that is already closed is a pass that changes nothing, which is what lets a commit run
    /// it without asking whether it has to.
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
