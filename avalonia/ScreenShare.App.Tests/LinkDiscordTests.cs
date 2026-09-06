using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ViewModel;
using ScreenShare.App.Features.Shell.Settings.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The Discord toggle carries the button that links this install,
/// so tying the group to a voice channel is reachable where the mode is switched on.
/// The secret lands in the stored settings on the backend's side,
/// which is why the press writes nothing here and the notice reads the session's Discord state.
/// </summary>
public sealed class LinkDiscordTests
{
    private static async Task<SetupViewModel> FlowAsync(SeededBackend backend)
    {
        var flow = Flows.Setup(backend);
        await flow.Settled;
        return flow;
    }

    /// <summary>Toggle control, reached by moving the flow to the step the form puts the relay on.</summary>
    private static FieldViewModel DiscordMode(SetupViewModel flow)
    {
        flow.CurrentStep = "relay";
        return flow.CurrentGroup!.Fields.Single(field => field.Key == RelayLayout.DiscordModeKey);
    }

    [Fact]
    public async Task TheToggleCarriesTheButtonThatLinks()
    {
        var flow = await FlowAsync(new SeededBackend("linux"));

        Assert.True(DiscordMode(flow).HasAction);
        Assert.Equal("Link Discord", DiscordMode(flow).Action!.Label);
        Assert.True(DiscordMode(flow).Action!.Command.CanExecute(null));
    }

    [Fact]
    public async Task APressRunsTheLinkAgainstTheDraftsRelay()
    {
        var backend = new SeededBackend("linux");
        var flow = await FlowAsync(backend);

        DiscordMode(flow).Action!.Command.Execute(null);
        await flow.Settled;

        var linked = Assert.Single(backend.DiscordLinks);
        Assert.Equal(backend.RelayHost, linked.Host);
    }

    /// <summary>
    /// The notice beside the button is the session's Discord state,
    /// so what the reader stands in is readable without pressing anything.
    /// </summary>
    [Fact]
    public async Task TheNoticeNamesTheChannelTheGroupFollows()
    {
        var backend = new SeededBackend("linux")
        {
            Discord = new DiscordState
            {
                Linked = true, InChannel = true,
                GuildName = "Guild", ChannelName = "General",
            },
        };
        var session = new Session(backend, action => action());
        var flow = Flows.Setup(backend, session);
        // The notice reads the session's Discord state, so the session has to have loaded it,
        // and the pass after that load is what composes the action row again.
        _ = session.Start();
        while (!session.IsLoaded)
        {
            await Task.Delay(1);
        }
        await flow.Settled;
        // The channel is what the mode follows, so the draft holds it on for the notice to name one.
        DiscordMode(flow).Flag = true;
        await flow.Settled;
        flow.Apply();

        Assert.True(DiscordMode(flow).HasActionNotice);
        Assert.Contains("General", DiscordMode(flow).ActionNotice);
        Assert.Contains("Guild", DiscordMode(flow).ActionNotice);
    }

    /// <summary>
    /// The toggle decides what follows a voice channel and decides nothing about the link,
    /// so the sentence beside the button reads the same on either side of it.
    /// </summary>
    [Fact]
    public async Task TheNoticeReadsTheSameOnEitherSideOfTheToggle()
    {
        var backend = new SeededBackend("linux")
        {
            Discord = new DiscordState { Linked = true, AccountName = "bjoern" },
        };
        var session = new Session(backend, action => action());
        var flow = Flows.Setup(backend, session);
        _ = session.Start();
        while (!session.IsLoaded)
        {
            await Task.Delay(1);
        }
        await flow.Settled;
        flow.Apply();
        var off = DiscordMode(flow).ActionNotice;

        DiscordMode(flow).Flag = true;
        await flow.Settled;
        flow.Apply();

        Assert.Equal(off, DiscordMode(flow).ActionNotice);
    }

    /// <summary>
    /// One fact, one sentence: the dialog and the button's notice draw the link from one place,
    /// so neither ends the clause its own way.
    /// </summary>
    [Fact]
    public async Task TheNoticeIsTheSentenceTheSettingsDialogDraws()
    {
        var backend = new SeededBackend("linux")
        {
            Discord = new DiscordState
            {
                Linked = true, AccountName = "bjoern", InChannel = true,
                GuildName = "Guild", ChannelName = "General",
            },
        };
        var session = new Session(backend, action => action());
        var flow = Flows.Setup(backend, session);
        _ = session.Start();
        while (!session.IsLoaded)
        {
            await Task.Delay(1);
        }
        await flow.Settled;
        flow.Apply();

        Assert.Equal(AppSettingsCopy.DiscordLine(backend.Discord), DiscordMode(flow).ActionNotice);
    }

    /// <summary>
    /// A refused link is held by this install and declined by the manager, two facts,
    /// so the notice states both rather than reading as an install that never linked.
    /// </summary>
    [Fact]
    public async Task ARefusedLinkReadsAsLinkedAndRefused()
    {
        var backend = new SeededBackend("linux")
        {
            Discord = new DiscordState { Linked = true, AccountName = "bjoern", LinkRefused = true },
        };
        var session = new Session(backend, action => action());
        var flow = Flows.Setup(backend, session);
        _ = session.Start();
        while (!session.IsLoaded)
        {
            await Task.Delay(1);
        }
        await flow.Settled;
        flow.Apply();

        Assert.Contains("bjoern", DiscordMode(flow).ActionNotice);
        Assert.Contains("does not recognize this link", DiscordMode(flow).ActionNotice);
        Assert.DoesNotContain("Not linked yet", DiscordMode(flow).ActionNotice);
        Assert.Equal("Link Discord again", DiscordMode(flow).Action!.Label);
        // A refusal blocks sharing, so the sentence is drawn in the failure hue
        // (docs/design-language.md, "Palette").
        Assert.True(DiscordMode(flow).ActionNoticeIsFailure);
    }

    /// <summary>
    /// A link the manager resolves says where sharing stands, which no hue answers,
    /// so the notice keeps the hint it carries everywhere else.
    /// </summary>
    [Fact]
    public async Task AResolvedLinkReadsAsAPlainNotice()
    {
        var backend = new SeededBackend("linux")
        {
            Discord = new DiscordState
            {
                Linked = true, AccountName = "bjoern", InChannel = true,
                GuildName = "Guild", ChannelName = "General",
            },
        };
        var session = new Session(backend, action => action());
        var flow = Flows.Setup(backend, session);
        _ = session.Start();
        while (!session.IsLoaded)
        {
            await Task.Delay(1);
        }
        await flow.Settled;
        flow.Apply();

        Assert.False(DiscordMode(flow).ActionNoticeIsFailure);
    }

    /// <summary>
    /// The account labels the link the settings hold, so it reads with the mode off,
    /// where no pass answers anything else about Discord.
    /// </summary>
    [Fact]
    public async Task TheNoticeNamesTheLinkedAccount()
    {
        var backend = new SeededBackend("linux")
        {
            Discord = new DiscordState { Linked = true, AccountName = "bjoern" },
        };
        var session = new Session(backend, action => action());
        var flow = Flows.Setup(backend, session);
        _ = session.Start();
        while (!session.IsLoaded)
        {
            await Task.Delay(1);
        }
        await flow.Settled;
        flow.Apply();

        Assert.Contains("bjoern", DiscordMode(flow).ActionNotice);
    }

    /// <summary>
    /// A linked install can press again, the press putting another account where the linked one stands,
    /// so the label says which of the two presses this is rather than offering a link already made.
    /// </summary>
    [Fact]
    public async Task ALinkedInstallIsOfferedAnotherAccount()
    {
        var backend = new SeededBackend("linux") { Discord = new DiscordState { Linked = true } };
        var session = new Session(backend, action => action());
        var flow = Flows.Setup(backend, session);
        _ = session.Start();
        while (!session.IsLoaded)
        {
            await Task.Delay(1);
        }
        await flow.Settled;
        flow.Apply();

        Assert.Equal("Link a different account", DiscordMode(flow).Action!.Label);
    }

    [Fact]
    public async Task AMachineWithNoRelayCannotLink()
    {
        var flow = await FlowAsync(new SeededBackend("linux") { RelayHost = "" });

        Assert.True(DiscordMode(flow).HasAction);
        Assert.False(DiscordMode(flow).Action!.Command.CanExecute(null));
    }
}
