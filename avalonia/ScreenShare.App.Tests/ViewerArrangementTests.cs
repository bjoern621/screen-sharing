using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Viewer.Model;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using ScreenShare.App.Features.Viewer.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The arrangement: which tile has focus, which streams are in windows of their own, and which
/// window is filling a screen.
///
/// None of it crosses the control contract, which is exactly why it is asserted here. The backend
/// describes decodes and a decode is not a tile, so there is no state on the wire that would
/// catch a shell whose focus and mode disagreed, whose pop-out left a window behind after the
/// stream was gone, or whose fullscreen named a tile that no longer existed
/// (<c>docs/ipc-api.md</c>).
/// </summary>
public sealed class ViewerArrangementTests
{
    private const string Leg = "rtsp";

    /// <summary>
    /// A backend carrying the named paths, whose form states the leg a tile is decoded on.
    ///
    /// Everything that is not about the relay or the viewer group is the seed's, and every effect
    /// answers at once, so a screen driven through a straight-through dispatcher has finished by
    /// the time a call returns.
    /// </summary>
    private sealed class ViewerBackend(params string[] paths) : IBackend
    {
        private readonly SeededBackend _seed = new("windows");

        public event Action? Changed
        {
            add { }
            remove { }
        }

        /// <summary>The decodes this backend is running, which the receive state is read back from.</summary>
        private readonly List<WatchKey> _decoding = [];

        public Task<RelayStatus> RelayStatusAsync(CancellationToken cancellation = default)
        {
            var relay = new RelayStatus { Reachable = true };
            foreach (var path in paths)
            {
                relay.Paths.Add(new RelayPath { Name = path, Ready = true });
            }

            return Task.FromResult(relay);
        }

        public Task<IReadOnlyList<ReceiveStream>> ReceivingAsync(CancellationToken cancellation = default)
            => Task.FromResult<IReadOnlyList<ReceiveStream>>(
                _decoding.Select(key => new ReceiveStream { Stream = key, Live = true }).ToList());

        /// <summary>A form carrying the one field the tile leg is read out of.</summary>
        public Task<Form> ResolveFormAsync(Settings draft, CancellationToken cancellation = default)
        {
            var form = new Form { Settings = draft.Clone() };
            var group = new FieldGroup { Key = "viewer" };
            group.Fields.Add(new Field
            {
                Key = "viewer.tile_watch_transport",
                Control = ControlKind.Select,
                Visible = true,
                Enabled = true,
                Value = new FieldValue { Text = Leg },
            });

            form.Groups.Add(group);
            return Task.FromResult(form);
        }

        public Task StartReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        {
            var key = new WatchKey { StreamName = streamName, Transport = transport };
            if (!_decoding.Contains(key))
            {
                _decoding.Add(key);
            }

            return Task.CompletedTask;
        }

        public Task StopReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        {
            _decoding.Remove(new WatchKey { StreamName = streamName, Transport = transport });
            return Task.CompletedTask;
        }

        public Task<Settings> SettingsAsync(CancellationToken cancellation = default)
            => _seed.SettingsAsync(cancellation);

        public Task<Catalog> CatalogAsync(CancellationToken cancellation = default)
            => _seed.CatalogAsync(cancellation);

        public Task<PublishState> PublishStateAsync(CancellationToken cancellation = default)
            => _seed.PublishStateAsync(cancellation);

        public Task<IReadOnlyList<WatchKey>> WatchingAsync(CancellationToken cancellation = default)
            => _seed.WatchingAsync(cancellation);

        public Task StartPublishAsync(Settings settings, CancellationToken cancellation = default)
            => _seed.StartPublishAsync(settings, cancellation);

        public Task SaveSettingsAsync(Settings settings, CancellationToken cancellation = default)
            => _seed.SaveSettingsAsync(settings, cancellation);

        public Task ApplyToStreamAsync(Settings settings, CancellationToken cancellation = default)
            => _seed.ApplyToStreamAsync(settings, cancellation);

        public Task StopPublishAsync(CancellationToken cancellation = default)
            => _seed.StopPublishAsync(cancellation);

        public Task<double> MeasureUplinkAsync(CancellationToken cancellation = default)
            => _seed.MeasureUplinkAsync(cancellation);

        public Task StartWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
            => _seed.StartWatchAsync(streamName, transport, cancellation);

        public Task StopWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
            => _seed.StopWatchAsync(streamName, transport, cancellation);

        public Task SetReceiveAudioAsync(
            string streamName, string transport, double volume, bool muted, CancellationToken cancellation = default)
            => _seed.SetReceiveAudioAsync(streamName, transport, volume, muted, cancellation);

        public Task<FrameChannel> OpenFramesAsync(string streamName, string transport, CancellationToken cancellation = default)
            => _seed.OpenFramesAsync(streamName, transport, cancellation);

        /// <summary>The arrangement this suite is about is the viewer's grid, which the publish's own preview is not in.</summary>
        public Task<FrameChannel> OpenPreviewFramesAsync(CancellationToken cancellation = default)
            => _seed.OpenPreviewFramesAsync(cancellation);

        public Task OpenLogAsync(string path, CancellationToken cancellation = default)
            => _seed.OpenLogAsync(path, cancellation);

        public Task OpenLogsFolderAsync(CancellationToken cancellation = default)
            => _seed.OpenLogsFolderAsync(cancellation);

        public IAsyncEnumerable<Event> SubscribeAsync(CancellationToken cancellation = default)
            => _seed.SubscribeAsync(cancellation);

        public IAsyncEnumerable<AudioLevels> SubscribeAudioLevelsAsync(CancellationToken cancellation = default)
            => _seed.SubscribeAudioLevelsAsync(cancellation);
    }

    /// <summary>
    /// A viewer with the given streams already tiled.
    ///
    /// The tiles are added the way the screen adds them - through the rail's own action - so what
    /// is under test is the state a real press leaves behind rather than a field written by hand.
    /// </summary>
    private static ViewerViewModel Grid(params string[] streams)
    {
        var backend = new ViewerBackend(streams);
        var session = new Session(backend, static action => action());
        session.Start();

        var viewer = Flows.Viewer(backend, session);
        viewer.Apply();

        foreach (var row in viewer.Streams.ToList())
        {
            row.Show.Execute(null);
        }

        return viewer;
    }

    private static TileViewModel Tile(ViewerViewModel viewer, string stream)
        => viewer.Tiles.Single(tile => tile.Name == stream);

    [Fact]
    public void FocusingATilePutsTheGridInFocusMode()
    {
        var viewer = Grid("one", "two");

        Tile(viewer, "one").ToggleFocus.Execute(null);

        Assert.Equal(LayoutMode.Focus, viewer.Mode);
        Assert.Equal("one", viewer.Focused);
        Assert.True(Tile(viewer, "one").IsFocused);
        Assert.False(Tile(viewer, "two").IsFocused);
    }

    /// <summary>
    /// A second tile asking for focus takes it. There is no state in which two are focused, so
    /// there is none in which one has to be given up first.
    /// </summary>
    [Fact]
    public void FocusMovesRatherThanAccumulating()
    {
        var viewer = Grid("one", "two");

        Tile(viewer, "one").ToggleFocus.Execute(null);
        Tile(viewer, "two").ToggleFocus.Execute(null);

        Assert.Equal("two", viewer.Focused);
        Assert.Single(viewer.Tiles, tile => tile.IsFocused);
    }

    /// <summary>Focusing the focused tile gives focus up, and the mode follows it back.</summary>
    [Fact]
    public void FocusingTheFocusedTileReturnsToTheGrid()
    {
        var viewer = Grid("one", "two");
        var tile = Tile(viewer, "one");

        tile.ToggleFocus.Execute(null);
        tile.ToggleFocus.Execute(null);

        Assert.Equal(LayoutMode.Grid, viewer.Mode);
        Assert.Equal("", viewer.Focused);
    }

    /// <summary>
    /// A popped-out stream keeps its tile and its place. The slot is what stops the grid
    /// reflowing when a stream pops out, and what it comes back into.
    /// </summary>
    [Fact]
    public void APoppedOutStreamKeepsItsSlot()
    {
        var viewer = Grid("one", "two");

        Tile(viewer, "one").TogglePopOut.Execute(null);

        Assert.Equal(2, viewer.Tiles.Count);
        Assert.True(Tile(viewer, "one").IsPoppedOut);
        Assert.Contains("one", viewer.PoppedOut);
    }

    /// <summary>Popping out does not disturb focus: they are different questions about one tile.</summary>
    [Fact]
    public void PoppingOutLeavesFocusAlone()
    {
        var viewer = Grid("one", "two");
        var tile = Tile(viewer, "one");

        tile.ToggleFocus.Execute(null);
        tile.TogglePopOut.Execute(null);

        Assert.Equal("one", viewer.Focused);
        Assert.Equal(LayoutMode.Focus, viewer.Mode);
        Assert.True(tile.IsPoppedOut);
    }

    /// <summary>
    /// Fullscreen goes to the window the stream is drawn in. A stream in the grid fills the main
    /// window; a popped-out one fills its own, which is what lets two of them fill two monitors.
    /// </summary>
    [Fact]
    public void FullscreenFollowsTheWindowTheStreamIsIn()
    {
        var viewer = Grid("one", "two");

        Tile(viewer, "one").ToggleFullscreen.Execute(null);
        Assert.Equal("one", viewer.Fullscreen);
        Assert.Empty(viewer.PoppedFullscreen);

        Tile(viewer, "two").TogglePopOut.Execute(null);
        Tile(viewer, "two").ToggleFullscreen.Execute(null);

        Assert.Equal("one", viewer.Fullscreen);
        Assert.Contains("two", viewer.PoppedFullscreen);
    }

    /// <summary>Two popped-out streams can be fullscreen at once, each on its own window.</summary>
    [Fact]
    public void TwoPoppedOutStreamsCanBothBeFullscreen()
    {
        var viewer = Grid("one", "two");

        foreach (var name in new[] { "one", "two" })
        {
            Tile(viewer, name).TogglePopOut.Execute(null);
            Tile(viewer, name).ToggleFullscreen.Execute(null);
        }

        Assert.Equal(2, viewer.PoppedFullscreen.Count);
    }

    /// <summary>
    /// A stream taken out of the grid takes its focus, its window and its fullscreen with it. The
    /// render pass works that out from the tile being gone rather than every caller undoing it,
    /// which is what stops a window outliving the tile behind it.
    /// </summary>
    [Fact]
    public void AStreamLeavingTheGridTakesItsArrangementWithIt()
    {
        var viewer = Grid("one", "two");
        var tile = Tile(viewer, "one");

        tile.ToggleFocus.Execute(null);
        tile.TogglePopOut.Execute(null);
        tile.ToggleFullscreen.Execute(null);

        viewer.Streams.Single(row => row.Name == "one").Show.Execute(null);

        Assert.Equal("", viewer.Focused);
        Assert.Equal(LayoutMode.Grid, viewer.Mode);
        Assert.DoesNotContain("one", viewer.PoppedOut);
        Assert.Empty(viewer.PoppedFullscreen);
    }

    /// <summary>
    /// The stats overlay turns on and stays on.
    ///
    /// It was once drawn by pinning the hover chrome, which meant turning it on and moving the
    /// pointer away turned it off again - useless for the one thing it exists for. It is its own
    /// state now, and this is what says so.
    /// </summary>
    [Fact]
    public void TheStatsOverlayIsItsOwnState()
    {
        var tile = Tile(Grid("one"), "one");

        Assert.False(tile.ShowStats);

        tile.ToggleStats.Execute(null);
        Assert.True(tile.ShowStats);

        tile.ToggleStats.Execute(null);
        Assert.False(tile.ShowStats);
    }

    /// <summary>
    /// The menu's words and glyphs follow the state they describe.
    ///
    /// Each item is one entry whose icon and wording change, rather than two entries one of which
    /// is hidden, so nothing in the menu moves under the pointer. The bindings read these, so a
    /// state that stopped updating them would be a menu quietly lying about what it will do.
    /// </summary>
    [Fact]
    public void TheMenuSaysWhatPressingItWillDo()
    {
        var viewer = Grid("one", "two");
        var tile = Tile(viewer, "one");

        var focus = tile.FocusGlyph;
        Assert.Equal("Focus", tile.FocusLabel);
        Assert.Equal("Pop out", tile.PopOutLabel);
        Assert.Equal("Stats overlay", tile.StatsLabel);

        tile.ToggleFocus.Execute(null);
        Assert.Equal("Leave focus", tile.FocusLabel);
        Assert.NotEqual(focus, tile.FocusGlyph);

        tile.TogglePopOut.Execute(null);
        Assert.Equal("Return to grid", tile.PopOutLabel);

        tile.ToggleStats.Execute(null);
        Assert.Equal("Hide stats", tile.StatsLabel);
    }

    /// <summary>
    /// Every intent is a toggle, so the same one twice is where it started. That is what makes
    /// the menu safe to press twice and the arrangement safe to describe as state.
    /// </summary>
    [Fact]
    public void EveryIntentIsAToggle()
    {
        var viewer = Grid("one");
        var tile = Tile(viewer, "one");

        tile.TogglePopOut.Execute(null);
        tile.TogglePopOut.Execute(null);

        Assert.Empty(viewer.PoppedOut);
        Assert.False(tile.IsPoppedOut);
    }
}
