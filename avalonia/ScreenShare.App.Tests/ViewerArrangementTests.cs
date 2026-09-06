using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Viewer.Model;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using ScreenShare.App.Features.Viewer.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Backend describes decodes and a decode is not a tile, so no state on the wire catches a focus and a mode
/// that disagree, a window outliving its stream, or a fullscreen naming a tile that is gone
/// (<c>docs/ipc-api.md</c>).
/// </summary>
public sealed class ViewerArrangementTests
{
    private const string Leg = "rtsp";

    /// <summary>
    /// A backend carrying the named relay paths.
    /// Leg is stated in the stored settings and in the form both:
    /// settings are what a decode opens on, the form is what the panel draws,
    /// and stating it once leaves a screen showing a leg no decode uses
    /// (Features/Viewer/Tile/Model/TileLeg.cs).
    /// Everything outside the relay and the viewer group is the seed's, and every effect answers at once.
    /// </summary>
    private sealed class ViewerBackend(params string[] paths) : IBackend
    {
        private readonly SeededBackend _seed = new("windows");

        public Task<string> VersionAsync(CancellationToken cancellation = default) => _seed.VersionAsync(cancellation);

        public event Action? Changed
        {
            add { }
            remove { }
        }

        private readonly List<StreamRef> _decoding = [];

        private string[] _paths = paths;

        /// <summary>
        /// Answer every start waits on, null while starts answer at once.
        /// The decode is open the moment it is asked for; the answer is what is late.
        /// </summary>
        private TaskCompletionSource? _heldStart;

        public void HoldStarts() => _heldStart = new TaskCompletionSource();

        /// <summary>Lets every held start answer, off the test's own context (<see cref="Answers.Now"/>).</summary>
        public void AnswerStarts()
        {
            var held = _heldStart;
            _heldStart = null;
            held?.SetResult();
        }

        /// <summary>
        /// Answers the held stops wait on, one per call, empty while stops answer at once.
        /// One per call rather than one shared: the runtime resumes a second awaiter of one task off the thread,
        /// and a test then reads a state still being written.
        /// </summary>
        private readonly Queue<TaskCompletionSource> _heldStops = [];

        private bool _holdsStops;

        public void HoldStops() => _holdsStops = true;

        /// <summary>Lets every held stop answer, in order, off the test's own context (<see cref="Answers.Now"/>).</summary>
        public void AnswerStops()
        {
            _holdsStops = false;
            while (_heldStops.TryDequeue(out var held))
            {
                held.SetResult();
            }
        }

        /// <summary>Starts asked for, a repeat included, so a pass that asks again is caught.</summary>
        public int Starts { get; private set; }

        /// <summary>Replaces what the relay lists, read on the next load.</summary>
        public void Carry(params string[] paths) => _paths = paths;

        /// <summary>Decodes still open, for asserting a stop reached the backend.</summary>
        public IReadOnlyList<StreamRef> Decoding => _decoding;

        public Task<RelayStatus> RelayStatusAsync(CancellationToken cancellation = default)
        {
            var relay = new RelayStatus { Reachable = true };
            foreach (var path in _paths)
            {
                relay.Paths.Add(new RelayPath { Name = path, Ready = true });
            }

            return Task.FromResult(relay);
        }

        public Task<IReadOnlyList<ReceiveStream>> ReceivingAsync(CancellationToken cancellation = default)
            => Task.FromResult<IReadOnlyList<ReceiveStream>>(
                _decoding.Select(streamRef => new ReceiveStream { Stream = streamRef, Live = true }).ToList());

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

        public Task StartReceiveAsync(
            string streamName, string transport, bool toneMap = false, CancellationToken cancellation = default)
        {
            var streamRef = new StreamRef { StreamName = streamName, Transport = transport };
            if (!_decoding.Contains(streamRef))
            {
                _decoding.Add(streamRef);
            }

            Starts++;
            return _heldStart?.Task ?? Task.CompletedTask;
        }

        public Task StopReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        {
            _decoding.Remove(new StreamRef { StreamName = streamName, Transport = transport });
            if (!_holdsStops)
            {
                return Task.CompletedTask;
            }

            var held = new TaskCompletionSource();
            _heldStops.Enqueue(held);
            return held.Task;
        }

        public async Task<Settings> SettingsAsync(CancellationToken cancellation = default)
        {
            var settings = await _seed.SettingsAsync(cancellation);
            settings.Viewer.TileWatchTransport = Leg;
            return settings;
        }

        public Task<Catalog> CatalogAsync(CancellationToken cancellation = default)
            => _seed.CatalogAsync(cancellation);

        public Task<PublishState> PublishStateAsync(CancellationToken cancellation = default)
            => _seed.PublishStateAsync(cancellation);

        public Task<IReadOnlyList<StreamRef>> WatchingAsync(CancellationToken cancellation = default)
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

    public Task<IReadOnlyList<RelayLeg>> CheckRelayAsync(Settings settings, CancellationToken cancellation = default)
            => _seed.CheckRelayAsync(settings, cancellation);



        public Task<(string Key, string Id)> CreateGroupAsync(RelaySettings relay, CancellationToken cancellation = default)
            => _seed.CreateGroupAsync(relay, cancellation);

        public Task<MembersState> MembersAsync(CancellationToken cancellation = default)
            => _seed.MembersAsync(cancellation);

        public Task<DiscordState> DiscordAsync(CancellationToken cancellation = default)
            => _seed.DiscordAsync(cancellation);


        public Task<string> ResolveLinkAsync(string url, CancellationToken cancellation = default)

            => _seed.ResolveLinkAsync(url, cancellation);

        public Task LinkDiscordAsync(RelaySettings relay, CancellationToken cancellation = default)
            => _seed.LinkDiscordAsync(relay, cancellation);

        public Task<TestStreamState> TestStreamsAsync(CancellationToken cancellation = default)
            => _seed.TestStreamsAsync(cancellation);

        public Task<PresetStore> PresetsAsync(CancellationToken cancellation = default)
            => _seed.PresetsAsync(cancellation);

        public Task SavePresetAsync(string name, PublishSettings settings, CancellationToken cancellation = default)
            => _seed.SavePresetAsync(name, settings, cancellation);

        public Task DeletePresetAsync(string name, CancellationToken cancellation = default)
            => _seed.DeletePresetAsync(name, cancellation);

        public Task StartWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
            => _seed.StartWatchAsync(streamName, transport, cancellation);

        public Task StopWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
            => _seed.StopWatchAsync(streamName, transport, cancellation);

        public Task OpenInBrowserAsync(string streamName, string transport, CancellationToken cancellation = default)
            => _seed.OpenInBrowserAsync(streamName, transport, cancellation);

        public Task SetReceiveAudioAsync(
            string streamName, string transport, double volume, bool muted, CancellationToken cancellation = default)
            => _seed.SetReceiveAudioAsync(streamName, transport, volume, muted, cancellation);

        public Task<FrameChannel> OpenFramesAsync(string streamName, string transport, CancellationToken cancellation = default)
            => _seed.OpenFramesAsync(streamName, transport, cancellation);

        public Task<FrameChannel> OpenPreviewFramesAsync(CancellationToken cancellation = default)
            => _seed.OpenPreviewFramesAsync(cancellation);

        public Task StartMonitorPreviewAsync(int monitor, CancellationToken cancellation = default)
            => _seed.StartMonitorPreviewAsync(monitor, cancellation);

        public Task StopMonitorPreviewAsync(int monitor, CancellationToken cancellation = default)
            => _seed.StopMonitorPreviewAsync(monitor, cancellation);

        public Task<FrameChannel> OpenMonitorFramesAsync(int monitor, CancellationToken cancellation = default)
            => _seed.OpenMonitorFramesAsync(monitor, cancellation);

        public Task<IReadOnlyList<PreviewedMonitor>> PreviewedMonitorsAsync(CancellationToken cancellation = default)
            => _seed.PreviewedMonitorsAsync(cancellation);

        public Task OpenLogAsync(string path, CancellationToken cancellation = default)
            => _seed.OpenLogAsync(path, cancellation);

        public Task OpenLogsFolderAsync(CancellationToken cancellation = default)
            => _seed.OpenLogsFolderAsync(cancellation);

        public Task<string> SendReportAsync(CancellationToken cancellation = default)
            => _seed.SendReportAsync(cancellation);

        public Task<UpdateState> UpdateAsync(CancellationToken cancellation = default)
            => _seed.UpdateAsync(cancellation);

        public Task CheckUpdateAsync(CancellationToken cancellation = default)
            => _seed.CheckUpdateAsync(cancellation);

        public Task InstallUpdateAsync(CancellationToken cancellation = default)
            => _seed.InstallUpdateAsync(cancellation);

        public IAsyncEnumerable<Event> SubscribeAsync(CancellationToken cancellation = default)
            => _seed.SubscribeAsync(cancellation);

        public IAsyncEnumerable<AudioLevels> SubscribeAudioLevelsAsync(CancellationToken cancellation = default)
            => _seed.SubscribeAudioLevelsAsync(cancellation);

        public IAsyncEnumerable<PointerPosition> SubscribePointerAsync(
            StreamRef? stream = null, CancellationToken cancellation = default)
            => _seed.SubscribePointerAsync(stream, cancellation);
    }

    /// <summary>
    /// A viewer with the given streams tiled through the rail's own action,
    /// so the state under test is what a real press leaves behind.
    /// </summary>
    private static ViewerViewModel Grid(params string[] streams) => GridOn(streams).Viewer;

    private static (ViewerViewModel Viewer, ViewerBackend Backend, Session Session) GridOn(
        params string[] streams)
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

        return (viewer, backend, session);
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

    /// <summary>No state holds two focused tiles, so none needs focus given up first.</summary>
    [Fact]
    public void FocusMovesRatherThanAccumulating()
    {
        var viewer = Grid("one", "two");

        Tile(viewer, "one").ToggleFocus.Execute(null);
        Tile(viewer, "two").ToggleFocus.Execute(null);

        Assert.Equal("two", viewer.Focused);
        Assert.Single(viewer.Tiles, tile => tile.IsFocused);
    }

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

    /// <summary>Slot stops the grid reflowing on a pop-out, and is what the stream comes back into.</summary>
    [Fact]
    public void APoppedOutStreamKeepsItsSlot()
    {
        var viewer = Grid("one", "two");

        Tile(viewer, "one").TogglePopOut.Execute(null);

        Assert.Equal(2, viewer.Tiles.Count);
        Assert.True(Tile(viewer, "one").IsPoppedOut);
        Assert.Contains("one", viewer.PoppedOut);
    }

    /// <summary>Focus and pop-out are different questions about one tile.</summary>
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

    /// <summary>Per-window fullscreen is what lets two streams fill two monitors.</summary>
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

    /// <summary>
    /// Every close runs this, the reconcile pass's own closes for a stream already given back included.
    /// One direction only: a toggle here asks for the window again,
    /// so leaving a pop-out from inside it opens a second one.
    /// </summary>
    [Fact]
    public void LeavingAPopOutIsOneDirectionAndIdempotent()
    {
        var viewer = Grid("one", "two");
        var tile = Tile(viewer, "one");

        tile.TogglePopOut.Execute(null);
        tile.ToggleFullscreen.Execute(null);

        tile.LeavePopOut.Execute(null);
        Assert.Empty(viewer.PoppedOut);
        Assert.False(tile.IsPoppedOut);

        // Window it filled went with it, so the stream comes back windowed.
        Assert.Empty(viewer.PoppedFullscreen);
        Assert.False(tile.IsFullscreen);

        // Close the reconcile pass performs for a stream already back in the grid.
        tile.LeavePopOut.Execute(null);
        Assert.Empty(viewer.PoppedOut);
        Assert.False(tile.IsPoppedOut);
    }

    /// <summary>
    /// Main window's fullscreen names a stream drawn in the main window.
    /// Left standing, it fills a screen with a plate saying the picture is in another window.
    /// </summary>
    [Fact]
    public void PoppingOutAFullscreenStreamGivesTheMainWindowBack()
    {
        var viewer = Grid("one", "two");
        var tile = Tile(viewer, "one");

        tile.ToggleFullscreen.Execute(null);
        tile.TogglePopOut.Execute(null);

        Assert.Equal("", viewer.Fullscreen);
        Assert.False(viewer.HasFullscreen);
        Assert.Null(viewer.FullscreenTile);

        // Fullscreen does not travel with the stream, so the window it went to arrives windowed.
        Assert.Empty(viewer.PoppedFullscreen);
        Assert.False(tile.IsFullscreen);
    }

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
    /// Render pass derives this from the tile being gone rather than each caller undoing it,
    /// so no window outlives its tile.
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

    /// <summary>Overlay is state of its own, not pinned hover chrome that goes off as the pointer leaves.</summary>
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
    /// Composing it while it is down costs rows per tile per sample for a panel nobody opened.
    /// Composing it on the next sample opens it empty for as long as a second, reading as a decode reporting nothing.
    /// </summary>
    [Fact]
    public void TheStatsPanelIsComposedOnlyWhileItIsUp()
    {
        var tile = Tile(Grid("one"), "one");

        Assert.Empty(tile.Stats);

        tile.ToggleStats.Execute(null);
        Assert.NotEmpty(tile.Stats);

        tile.ToggleStats.Execute(null);
        Assert.Empty(tile.Stats);
    }

    /// <summary>
    /// A row's words are fixed and its state is the tick beside them,
    /// so the bindings carry the flag and never a rewritten label.
    /// </summary>
    [Fact]
    public void TheMenuReportsWhatIsInForce()
    {
        var viewer = Grid("one", "two");
        var tile = Tile(viewer, "one");

        Assert.False(tile.IsFocused);
        Assert.False(tile.IsPoppedOut);
        Assert.False(tile.IsFullscreen);
        Assert.False(tile.ShowStats);

        tile.ToggleFocus.Execute(null);
        Assert.True(tile.IsFocused);

        tile.ToggleStats.Execute(null);
        Assert.True(tile.ShowStats);

        // Fullscreen belongs to the window a stream is drawn in,
        // so the flag follows the stream across both rather than answering for the main window.
        tile.ToggleFullscreen.Execute(null);
        Assert.True(tile.IsFullscreen);

        tile.TogglePopOut.Execute(null);
        Assert.True(tile.IsPoppedOut);
        Assert.False(tile.IsFullscreen);

        tile.ToggleFullscreen.Execute(null);
        Assert.True(tile.IsFullscreen);
    }

    /// <summary>Makes a menu row safe to press twice and the arrangement safe to hold as state.</summary>
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

    /// <summary>Each popped-out window fills a screen of its own and answers for it.</summary>
    [Fact]
    public void LeavingFullscreenFreesTheMainWindowAlone()
    {
        var viewer = Grid("one", "two");

        Tile(viewer, "one").ToggleFullscreen.Execute(null);
        Tile(viewer, "two").TogglePopOut.Execute(null);
        Tile(viewer, "two").ToggleFullscreen.Execute(null);

        viewer.LeaveFullscreen.Execute(null);

        Assert.Equal("", viewer.Fullscreen);
        Assert.False(Tile(viewer, "one").IsFullscreen);
        Assert.Contains("two", viewer.PoppedFullscreen);
        Assert.True(Tile(viewer, "two").IsFullscreen);
    }

    /// <summary>
    /// The rail lists what the relay carries, so a dropped stream has no row left to close from.
    /// The tile's own close is the way out the reader still has.
    /// </summary>
    [Fact]
    public void ATileOutlivingItsRowStillCloses()
    {
        var (viewer, backend, session) = GridOn("one");

        backend.Carry();
        session.Start();
        viewer.Apply();

        Assert.Empty(viewer.Streams);
        var tile = Tile(viewer, "one");

        tile.Close.Execute(null);

        Assert.Empty(viewer.Tiles);
        Assert.Empty(backend.Decoding);
    }

    /// <summary>Close names the state out of the grid, so a second press finds it holds and does nothing.</summary>
    [Fact]
    public void ClosingATileIsOneDirectionAndIdempotent()
    {
        var (viewer, backend, _) = GridOn("one");
        var tile = Tile(viewer, "one");

        tile.Close.Execute(null);
        tile.Close.Execute(null);

        Assert.Empty(viewer.Tiles);
        Assert.Empty(backend.Decoding);
        Assert.False(viewer.Streams.Single(row => row.Name == "one").IsTiled);
    }

    /// <summary>Names a state rather than toggling one, so it is safe on a key that fires on every screen.</summary>
    [Fact]
    public void LeavingFullscreenNeverEntersIt()
    {
        var viewer = Grid("one");
        var tile = Tile(viewer, "one");

        viewer.LeaveFullscreen.Execute(null);
        tile.LeaveFullscreen.Execute(null);

        Assert.Equal("", viewer.Fullscreen);
        Assert.Empty(viewer.PoppedFullscreen);
        Assert.False(tile.IsFullscreen);

        viewer.LeaveFullscreen.Execute(null);

        Assert.Equal("", viewer.Fullscreen);
    }

    /// <summary>A popped-out stream stops filling its screen and stays in its window.</summary>
    [Fact]
    public void LeavingFullscreenFollowsTheWindowTheStreamIsIn()
    {
        var viewer = Grid("one");
        var tile = Tile(viewer, "one");

        tile.TogglePopOut.Execute(null);
        tile.ToggleFullscreen.Execute(null);
        tile.LeaveFullscreen.Execute(null);

        Assert.Empty(viewer.PoppedFullscreen);
        Assert.Contains("one", viewer.PoppedOut);
        Assert.False(tile.IsFullscreen);

        // And the main window's, for a stream in the grid.
        tile.TogglePopOut.Execute(null);
        tile.ToggleFullscreen.Execute(null);
        tile.LeaveFullscreen.Execute(null);

        Assert.Equal("", viewer.Fullscreen);
        Assert.False(tile.IsFullscreen);
    }

    /// <summary>
    /// A decode runs while a window draws its tile.
    /// A window closed to the tray draws nothing, so what its grid held is a relay client and a decoder running
    /// for nobody (<c>docs/viewer-architecture.md</c>, "A decode runs while a window draws it").
    /// </summary>
    [Fact]
    public void HidingTheWindowStopsEveryDecodeItsGridDrew()
    {
        var (viewer, backend, _) = GridOn("one", "two");

        viewer.SetWindowShown(false);

        Assert.Empty(viewer.Tiles);
        Assert.Empty(backend.Decoding);
    }

    /// <summary>A pop-out window stays on screen with the main window hidden, so the decode it draws runs on.</summary>
    [Fact]
    public void APoppedOutStreamKeepsItsDecodeWhileTheWindowIsHidden()
    {
        var (viewer, backend, _) = GridOn("one", "two");
        Tile(viewer, "one").TogglePopOut.Execute(null);

        viewer.SetWindowShown(false);

        Assert.Single(viewer.Tiles, tile => tile.Name == "one");
        Assert.Single(backend.Decoding, decode => decode.StreamName == "one");
        Assert.Contains("one", viewer.PoppedOut);
    }

    /// <summary>
    /// The slot a closed pop-out returns to is in a window nothing shows, and the stream is back in the grid
    /// when the window is.
    /// </summary>
    [Fact]
    public void APopOutClosedWhileTheWindowIsHiddenStopsItsDecodeUntilTheWindowIsBack()
    {
        var (viewer, backend, _) = GridOn("one");
        var tile = Tile(viewer, "one");
        tile.TogglePopOut.Execute(null);
        viewer.SetWindowShown(false);

        tile.LeavePopOut.Execute(null);

        Assert.Empty(viewer.Tiles);
        Assert.Empty(backend.Decoding);

        viewer.SetWindowShown(true);

        Assert.Single(viewer.Tiles, back => back.Name == "one");
        Assert.Single(backend.Decoding);
        Assert.Empty(viewer.PoppedOut);
    }

    /// <summary>
    /// What the grid watched is the reader's arrangement and survives the hide,
    /// so the window comes back watching it, a start per stream answered as any press is.
    /// Stating the same visibility twice asks for nothing more.
    /// </summary>
    [Fact]
    public void ShowingTheWindowAgainWatchesWhatItWatched()
    {
        var (viewer, backend, _) = GridOn("one", "two");
        viewer.SetWindowShown(false);
        Assert.Empty(viewer.Tiles);

        viewer.SetWindowShown(true);
        viewer.SetWindowShown(true);

        Assert.Equal(["one", "two"], viewer.Tiles.Select(tile => tile.Name));
        Assert.Equal(2, backend.Decoding.Count);
        Assert.Equal(4, backend.Starts);
    }

    /// <summary>
    /// A quit waits on this, so the answer is the backend's rather than the hide's own return:
    /// a stop still on its way as the process exits is a decode left running on a backend that outlives the shell.
    /// </summary>
    [Fact]
    public void PartingClosesEveryDecodeAndAnswersOnceTheBackendHas()
    {
        var (viewer, backend, _) = GridOn("one", "two");
        backend.HoldStops();

        var parted = viewer.PartAsync();

        Assert.Empty(viewer.Tiles);
        Assert.False(parted.IsCompleted);

        Answers.Now(backend.AnswerStops);

        Assert.True(parted.IsCompleted);
        Assert.Empty(backend.Decoding);
        Assert.True(viewer.PartAsync().IsCompleted);
    }

    /// <summary>
    /// A start is a round trip, and the window can hide while the answer is out.
    /// A tile added then would sit in a window nothing shows, holding the decode the hide was meant to end.
    /// </summary>
    [Fact]
    public void AStartAnsweringIntoAHiddenWindowClosesItsDecodeAgain()
    {
        var backend = new ViewerBackend("one");
        var session = new Session(backend, static action => action());
        session.Start();
        var viewer = Flows.Viewer(backend, session);
        viewer.Apply();

        backend.HoldStarts();
        viewer.Streams.Single().Show.Execute(null);
        Assert.Single(backend.Decoding);

        viewer.SetWindowShown(false);
        Answers.Now(backend.AnswerStarts);

        Assert.Empty(viewer.Tiles);
        Assert.Empty(backend.Decoding);

        // Kept like the tiles the hide took down, so the press is honoured once there is a window for it.
        viewer.SetWindowShown(true);

        Assert.Single(viewer.Tiles);
        Assert.Single(backend.Decoding);
    }
}
