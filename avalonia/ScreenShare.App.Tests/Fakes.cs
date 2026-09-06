using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Shell.Update.ViewModel;
using ScreenShare.App.Features.Setup.ViewModel;
using ScreenShare.App.Features.Viewer.ViewModel;

namespace ScreenShare.App.Tests;

/// <summary>
/// Answering a held call so that everything it sets off, the awaiting call resuming, the dispatch,
/// the adoption and the render pass, has happened by the time the answer returns.
/// A test then asserts against a settled state rather than against a race.
/// </summary>
internal static class Answers
{
    /// <summary>
    /// Completes a held call with the test framework's synchronization context off the thread.
    /// The runtime refuses to resume an awaiting continuation inline on a thread carrying a context
    /// and queues it to the thread pool, leaving the view model rendering on one thread
    /// while the test reads its properties on another, in no order awaiting can fix.
    /// With the context off, the resumption runs here.
    /// </summary>
    public static void Now(Action complete)
    {
        var context = SynchronizationContext.Current;
        SynchronizationContext.SetSynchronizationContext(null);
        try
        {
            complete();
        }
        finally
        {
            SynchronizationContext.SetSynchronizationContext(context);
        }
    }
}

/// <summary>
/// Backend whose resolves are answered by hand, the only way to write down the real one's timing:
/// a socket lets two drafts be in flight at once and puts no order on their answers.
/// Every resolve is held and answered from the seeded form on request, in whichever order a test chooses.
/// The token each call was given is kept too, so a test can state that superseding a draft asked the older
/// call to stop rather than merely ignoring its answer.
/// </summary>
internal sealed class DeferredBackend : IBackend
{
    private sealed record Held(Settings Draft, TaskCompletionSource<Form> Answer, CancellationToken Cancellation);

    /// <summary>Sentence a read fails with when nothing is listening, worded as the client words it.</summary>
    private const string Absent = "The backend is not running: nothing is listening on the control socket.";

    private readonly SeededBackend _seed = new("linux");
    private readonly List<Held> _held = [];
    private readonly List<TaskCompletionSource> _heldSaves = [];
    private TaskCompletionSource _saveAsked = new();

    /// <summary>
    /// Stands in for the encoder probe landing, what the real backend raises this for.
    /// Raised by hand, so a test can state that news of a moved answer makes the flow read again
    /// rather than redraw what it holds.
    /// </summary>
    public event Action? Changed;

    /// <summary>
    /// Whether reads fail outright instead of being held: no call to answer later,
    /// the state a window that opened before its backend is in.
    /// False for a backend that has come up.
    /// </summary>
    public bool IsAbsent { get; set; }

    /// <summary>Resolves asked for, the count an idempotent pass leaves alone.</summary>
    public int Resolves => _held.Count;

    /// <summary>Discord mode as a manager pass read it, written by a test that moves the channel.</summary>
    public DiscordState Discord
    {
        get => _seed.Discord;
        set => _seed.Discord = value;
    }

    public void Announce() => Changed?.Invoke();

    public Task<Catalog> CatalogAsync(CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.CatalogAsync(cancellation);

    public Task<Settings> SettingsAsync(CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.SettingsAsync(cancellation);

    public Task<string> VersionAsync(CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.VersionAsync(cancellation);

    public Task<Form> ResolveFormAsync(Settings draft, CancellationToken cancellation = default)
    {
        if (IsAbsent)
        {
            throw new BackendUnavailableException(Absent);
        }

        var held = new Held(draft.Clone(), new TaskCompletionSource<Form>(), cancellation);
        _held.Add(held);
        return held.Answer.Task;
    }

    // Only one method's timing is this stand-in's business, so the rest of the interface forwards to the seed.
    // Two sets of answers would be two fixtures to keep in step.

    public Task<PublishState> PublishStateAsync(CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.PublishStateAsync(cancellation);

    public Task<RelayStatus> RelayStatusAsync(CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.RelayStatusAsync(cancellation);

    public Task<IReadOnlyList<StreamRef>> WatchingAsync(CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.WatchingAsync(cancellation);

    public Task StartPublishAsync(Settings settings, CancellationToken cancellation = default)
        => _seed.StartPublishAsync(settings, cancellation);

    public Task ApplyToStreamAsync(Settings settings, CancellationToken cancellation = default)
        => _seed.ApplyToStreamAsync(settings, cancellation);

    /// <summary>
    /// Write, answered at once unless <see cref="DefersSaves"/> is set,
    /// in which case it is held like a resolve and for the same reason:
    /// a socket lets a second write be asked for while the first is unanswered.
    /// What was handed over is recorded either way.
    /// </summary>
    public Task SaveSettingsAsync(Settings settings, CancellationToken cancellation = default)
    {
        if (IsAbsent)
        {
            throw new BackendUnavailableException(Absent);
        }

        Saved.Add(settings.Clone());

        // Whoever waited for a write has had one, and the next waiter gets a fresh source,
        // so a wait is always for a write still to come.
        var asked = _saveAsked;
        _saveAsked = new TaskCompletionSource();
        asked.SetResult();

        if (!DefersSaves)
        {
            return _seed.SaveSettingsAsync(settings, cancellation);
        }

        var answer = new TaskCompletionSource();
        _heldSaves.Add(answer);
        return answer.Task;
    }

    /// <summary>
    /// Completes when the next write is asked for, what a test waits on instead of sleeping.
    /// A held write's continuation runs on whichever thread completed it,
    /// so the count alone says nothing about whether it has run.
    /// </summary>
    public Task NextSaveAsked => _saveAsked.Task;

    public bool DefersSaves { get; set; }

    /// <summary>Settings each write was given, oldest first.</summary>
    public List<Settings> Saved { get; } = [];

    public int HeldSaves => _heldSaves.Count(answer => !answer.Task.IsCompleted);

    /// <summary>Answers the oldest unanswered write, as a backend that stored it.</summary>
    public void AnswerSave()
    {
        var answer = _heldSaves.First(held => !held.Task.IsCompleted);
        answer.SetResult();
    }

    public Task StopPublishAsync(CancellationToken cancellation = default)
        => _seed.StopPublishAsync(cancellation);

    public Task<double> MeasureUplinkAsync(CancellationToken cancellation = default)
        => _seed.MeasureUplinkAsync(cancellation);

    public Task<IReadOnlyList<RelayLeg>> CheckRelayAsync(Settings settings, CancellationToken cancellation = default)
        => _seed.CheckRelayAsync(settings, cancellation);



    public Task<(string Key, string Id)> CreateGroupAsync(RelaySettings relay, CancellationToken cancellation = default)
        => _seed.CreateGroupAsync(relay, cancellation);

    public Task<MembersState> MembersAsync(CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.MembersAsync(cancellation);

    public Task<DiscordState> DiscordAsync(CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.DiscordAsync(cancellation);

    public Task<string> ResolveLinkAsync(string url, CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.ResolveLinkAsync(url, cancellation);

    public Task LinkDiscordAsync(RelaySettings relay, CancellationToken cancellation = default)
        => _seed.LinkDiscordAsync(relay, cancellation);

    public Task<TestStreamState> TestStreamsAsync(CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.TestStreamsAsync(cancellation);

    public Task<PresetStore> PresetsAsync(CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.PresetsAsync(cancellation);

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

    public Task<IReadOnlyList<ReceiveStream>> ReceivingAsync(CancellationToken cancellation = default)
        => _seed.ReceivingAsync(cancellation);

    public Task StartReceiveAsync(
        string streamName, string transport, bool toneMap = false, CancellationToken cancellation = default)
        => _seed.StartReceiveAsync(streamName, transport, toneMap, cancellation);

    public Task StopReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        => _seed.StopReceiveAsync(streamName, transport, cancellation);

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

    /// <summary>Draft one held resolve was asked about, indexed in the order the resolves arrived.</summary>
    public Settings Draft(int resolve) => _held[resolve].Draft;

    /// <summary>
    /// What the backend walks a draft to before answering about it, null for one taking drafts as they come.
    /// The real one repairs a value no rule allows (<c>backend/internal/form/repair.go</c>),
    /// and a repair landing while the reader holds a thumb is what a sweep has to survive.
    /// </summary>
    public Action<Settings>? Repairs { get; set; }

    public bool IsCancelled(int resolve) => _held[resolve].Cancellation.IsCancellationRequested;

    /// <summary>
    /// Answers one held resolve with the form its own draft resolves to.
    /// Everything the answer sets off has happened on return, for the reason <see cref="Answers.Now"/> states.
    /// </summary>
    public async Task AnswerAsync(int resolve)
    {
        var held = _held[resolve];
        var asked = held.Draft.Clone();
        Repairs?.Invoke(asked);

        var form = await _seed.ResolveFormAsync(asked);

        Answers.Now(() => held.Answer.SetResult(form));
    }

    /// <summary>
    /// Refuses one held resolve as an absent backend does, with the sentence the screen shows.
    /// Everything it sets off has happened on return, for the reason <see cref="Answers.Now"/> states.
    /// </summary>
    public void Fail(int resolve, string reason)
        => Answers.Now(() => _held[resolve].Answer.SetException(new BackendUnavailableException(reason)));
}

/// <summary>
/// Backend whose running state a test writes: what the relay answered, what is publishing,
/// and whether a commit is accepted or refused.
/// <see cref="SeededBackend"/> seeds no relay and no pipeline, and those are what every commit condition
/// reads, down to the one deciding which effect the press is.
/// No form is answered here: the resolve stays the seed's, so the settings half of the gate has one fixture
/// behind it rather than a second copy of the domain.
/// </summary>
internal sealed class PublishingBackend : IBackend
{
    private readonly SeededBackend _seed = new("linux");

    public Task<string> VersionAsync(CancellationToken cancellation = default) => _seed.VersionAsync(cancellation);

    public event Action? Changed
    {
        add { }
        remove { }
    }

    /// <summary>Relay snapshot. Reachable until a test writes the failure it is about.</summary>
    public RelayStatus Relay { get; set; } = new() { Reachable = true };

    /// <summary>What is publishing. Nothing until a test writes one, which the absent <c>Live</c> says.</summary>
    public PublishState Publish { get; set; } = new();

    /// <summary>
    /// Stream built from the settings this backend holds, which is the draft a fresh flow opens on.
    /// Written into <see cref="Publish"/> by a test that wants the running pipeline to be the drafted one,
    /// so the resolve answers <c>Form.in_force</c>.
    /// </summary>
    public PublishState Running()
    {
        var settings = SettingsAsync().Result;

        return new PublishState
        {
            Live = new PublishState.Types.Live
            {
                Publish = settings.Publish,
                Relay = settings.Relay,
                StreamName = settings.StreamName,
            },
        };
    }

    /// <summary>Refusal every commit meets, empty while commits go through.</summary>
    public string Refusal { get; set; } = "";

    /// <summary>Settings each accepted start was given, oldest first.</summary>
    public List<Settings> Started { get; } = [];

    /// <summary>
    /// Settings each accepted apply was given, oldest first.
    /// Kept apart from <see cref="Started"/>, which list a commit lands in being the whole question:
    /// the backend refuses each effect in the state the other one is for.
    /// </summary>
    public List<Settings> Applied { get; } = [];

    /// <summary>
    /// Commit asked for and not answered, turning the round trip into an interval a test can read the screen
    /// in the middle of.
    /// Every other answer here is immediate,
    /// so the rest of the tests are about what the screen says rather than about timing.
    /// </summary>
    private TaskCompletionSource? _held;

    /// <summary>Holds every commit from here on, so one can be read while it is in flight.</summary>
    public void HoldStarts() => _held = new TaskCompletionSource();

    /// <summary>
    /// Answers the held commit.
    /// Everything it sets off has happened on return, for the reason <see cref="Answers.Now"/> states.
    /// </summary>
    public void AnswerStarts()
    {
        var held = _held ?? throw new InvalidOperationException("no start is being held");

        _held = null;
        Answers.Now(held.SetResult);
    }

    public Task StartPublishAsync(Settings settings, CancellationToken cancellation = default)
        => Commit(Started, settings);

    public Task ApplyToStreamAsync(Settings settings, CancellationToken cancellation = default)
        => Commit(Applied, settings);

    public Task SaveSettingsAsync(Settings settings, CancellationToken cancellation = default)
        => Commit(Saved, settings);

    /// <summary>Settings each write was given, oldest first.</summary>
    public List<Settings> Saved { get; } = [];

    /// <summary>
    /// One commit, recorded where a test looks for it.
    /// Both effects carry the whole draft, answer with nothing and are refused or held on the same terms,
    /// so which list the settings landed in is all there is to read.
    /// </summary>
    private Task Commit(List<Settings> into, Settings settings)
    {
        if (Refusal.Length > 0)
        {
            // Faulted rather than thrown, so the caller's await raises it,
            // the path the gRPC client puts a refusal on.
            return Task.FromException(new BackendUnavailableException(Refusal));
        }

        into.Add(settings);
        return _held?.Task ?? Task.CompletedTask;
    }

    public Task<PublishState> PublishStateAsync(CancellationToken cancellation = default)
        => Task.FromResult(Publish);

    public Task<RelayStatus> RelayStatusAsync(CancellationToken cancellation = default)
        => Task.FromResult(Relay);

    // Everything past this point forwards, as DeferredBackend does.

    public Task<Catalog> CatalogAsync(CancellationToken cancellation = default)
        => _seed.CatalogAsync(cancellation);

    public Task<Settings> SettingsAsync(CancellationToken cancellation = default)
        => _seed.SettingsAsync(cancellation);

    /// <summary>
    /// The seed's form, carrying this backend's verdict on whether the draft is what publishes:
    /// the two groups the stream was built from equal the draft's.
    /// Field equality stands in for the backend's render comparison (<c>publish.SamePipeline</c>),
    /// the shell reading the verdict alone.
    /// </summary>
    public async Task<Form> ResolveFormAsync(Settings draft, CancellationToken cancellation = default)
    {
        var form = await _seed.ResolveFormAsync(draft, cancellation).ConfigureAwait(false);

        form.InForce = Publish.Live is { } live
            && Equals(live.Publish, form.Settings.Publish)
            && Equals(live.Relay, form.Settings.Relay);

        return form;
    }

    public Task<IReadOnlyList<StreamRef>> WatchingAsync(CancellationToken cancellation = default)
        => _seed.WatchingAsync(cancellation);

    public Task StopPublishAsync(CancellationToken cancellation = default)
    {
        if (Refusal.Length > 0)
        {
            return Task.FromException(new BackendUnavailableException(Refusal));
        }

        Stopped++;
        return Task.CompletedTask;
    }

    /// <summary>Accepted stops, counted: a stop carries nothing worth recording beside how often.</summary>
    public int Stopped { get; private set; }

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

    public Task<IReadOnlyList<ReceiveStream>> ReceivingAsync(CancellationToken cancellation = default)
        => _seed.ReceivingAsync(cancellation);

    public Task StartReceiveAsync(
        string streamName, string transport, bool toneMap = false, CancellationToken cancellation = default)
        => _seed.StartReceiveAsync(streamName, transport, toneMap, cancellation);

    public Task StopReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        => _seed.StopReceiveAsync(streamName, transport, cancellation);

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
/// Two destinations that read the window's one settings draft, built as the window builds them:
/// one <see cref="Session"/> and one <see cref="FormSession"/> behind both.
/// The window holds one draft for the whole app, the wizard writing what this machine sends
/// and the viewer how it receives, into the same message,
/// so a fixture handing each its own would test an arrangement the app does not have.
/// </summary>
internal static class Flows
{
    /// <summary>Inline, so a render pass is over when the call is.</summary>
    private static readonly Action<Action> Inline = action => action();

    public static SetupViewModel Setup(IBackend backend, Session session)
        => new(backend, new FormSession(backend, session, Inline), session, Inline);

    public static SetupViewModel Setup(IBackend backend) => Setup(backend, new Session(backend, Inline));

    public static ViewerViewModel Viewer(IBackend backend, Session session)
        => new(backend, new FormSession(backend, session, Inline), session, Inline);

    /// <summary>
    /// What the app says about the published release, over a session that has read once.
    /// The read is what fills the state in: a session that never started answers nothing,
    /// which is the band with no version behind it rather than the one under test.
    /// </summary>
    public static UpdateViewModel Updates(IBackend backend)
    {
        var session = new Session(backend, Inline);
        session.Start();
        session.Stop();
        return new UpdateViewModel(backend, session, Inline);
    }

    public static UpdateViewModel Updates(IBackend backend, Session session)
        => new(backend, session, Inline);
}
