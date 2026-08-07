using System.Collections.Specialized;
using System.ComponentModel;
using ScreenShare.App.Relay;
using ScreenShare.App.Ui;
using Xunit;

namespace ScreenShare.App.Tests;

public sealed class MainViewModelTests
{
    private static readonly RelayTarget Target = new("127.0.0.1", 9997, TimeSpan.FromSeconds(2));

    private const string OnePath = """
        {"items":[{"name":"bjoern","ready":true,"tracks":["H264"],
        "bytesReceived":0,"readers":[{"type":"webrtcSession"}]}]}
        """;

    /// <summary>
    /// The poller dispatches straight through here, so a completed poll has rendered by
    /// the time the awaiting test resumes.
    /// </summary>
    private static (MainViewModel Model, RelayPoller Poller) Build(
        Func<HttpRequestMessage, HttpResponseMessage> answer)
    {
        var http = new HttpClient(new StubHandler(answer)) { Timeout = TimeSpan.FromSeconds(3) };
        var poller = new RelayPoller(new RelayClient(http, new StubClock()), action => action());
        var model = new MainViewModel(poller) { AutoRefresh = false };
        model.Start();
        return (model, poller);
    }

    [Fact]
    public void BeforeTheFirstPollTheViewIsIdleAndEmpty()
    {
        var (model, _) = Build(_ => StubHandler.Json(OnePath));

        Assert.Equal("Not checked yet", model.StatusLabel);
        Assert.False(model.HasError);
        Assert.Equal("", model.ErrorText);
        Assert.True(model.IsEmpty);
        Assert.Empty(model.Paths);
        Assert.True(model.CanCheck);
    }

    [Fact]
    public async Task AReachableRelayListsItsPathsAndCountsThem()
    {
        var (model, poller) = Build(_ => StubHandler.Json(OnePath));

        await poller.CheckOnceAsync(Target);

        Assert.Equal("Connected", model.StatusLabel);
        Assert.False(model.HasError);
        Assert.False(model.IsEmpty);
        Assert.Equal("1 live · 1 watching", model.Summary);

        var row = Assert.Single(model.Paths);
        Assert.Equal("bjoern", row.Name);
        Assert.Equal("h264", row.Format);
        Assert.Equal("1 watching", row.Readers);
        Assert.Equal("…", row.Bitrate);
    }

    [Fact]
    public async Task AFailedCheckShowsTheReasonAndTheFailedFace()
    {
        var (model, poller) = Build(_ => throw new HttpRequestException("connection refused"));

        await poller.CheckOnceAsync(Target);

        Assert.Equal("No answer from the relay", model.StatusLabel);
        Assert.True(model.HasError);
        Assert.Contains("connection refused", model.ErrorText);
        Assert.True(model.IsEmpty);
    }

    [Fact]
    public async Task ARecoveredRelayClearsTheErrorTheFailureLeft()
    {
        var reachable = false;
        var (model, poller) = Build(_ => reachable
            ? StubHandler.Json(OnePath)
            : throw new HttpRequestException("connection refused"));

        await poller.CheckOnceAsync(Target);
        Assert.True(model.HasError);

        reachable = true;
        await poller.CheckOnceAsync(Target);

        // The render function sets the off branch too, so nothing of the failure sticks.
        Assert.False(model.HasError);
        Assert.Equal("", model.ErrorText);
        Assert.Equal("Connected", model.StatusLabel);
    }

    [Fact]
    public async Task RenderingTwiceOverUnchangedStateChangesNothing()
    {
        var (model, poller) = Build(_ => StubHandler.Json(OnePath));
        await poller.CheckOnceAsync(Target);

        var properties = new List<string?>();
        var collectionChanges = 0;
        model.PropertyChanged += (_, args) => properties.Add(args.PropertyName);
        ((INotifyCollectionChanged)model.Paths).CollectionChanged += (_, _) => collectionChanges++;

        model.Apply();

        Assert.Empty(properties);
        Assert.Equal(0, collectionChanges);
    }

    [Fact]
    public void AnEmptiedPortLeavesNothingToCheck()
    {
        var (model, _) = Build(_ => StubHandler.Json(OnePath));

        model.ApiPort = null;

        Assert.False(model.CanCheck);
        Assert.False(model.CheckNowCommand.CanExecute(null));
    }

    [Fact]
    public void AnEmptiedHostLeavesNothingToCheck()
    {
        var (model, _) = Build(_ => StubHandler.Json(OnePath));

        model.Host = "   ";

        Assert.False(model.CanCheck);
    }
}
