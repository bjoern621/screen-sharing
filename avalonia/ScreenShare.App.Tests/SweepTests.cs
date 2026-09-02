using Avalonia.Controls.Primitives;
using Avalonia.Input;
using ScreenShare.App.Backend;
using ScreenShare.App.Controls;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Viewer.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A control under a held thumb, against a backend that answers by hand.
/// A sweep writes a value per pointer move, each of them a configuration the reader is passing through.
/// Three different answers to that: what the draft takes, what the screen prints, what the backend is asked
/// (<c>docs/settings-editing.md</c>).
/// </summary>
public sealed class SweepTests
{
    private static readonly Action<Action> Inline = action => action();

    private const string Latency = "viewer.srt_watch_latency_ms";

    private sealed record Panel(ViewerViewModel Viewer, FormSession Form, DeferredBackend Backend);

    /// <summary>Watch panel on an answered opening read, where the sliders are.</summary>
    private static async Task<Panel> PanelAsync()
    {
        var backend = new DeferredBackend();
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        var viewer = new ViewerViewModel(backend, form, session, Inline);

        await backend.AnswerAsync(0);
        return new Panel(viewer, form, backend);
    }

    private static FieldViewModel Field(Panel panel, string key)
        => panel.Viewer.Watch.Group.Fields.Single(field => field.Key == key);

    /// <summary>
    /// The whole of the sweep: the draft follows the thumb, the backend is asked as it moves,
    /// and each answer is followed by a question about wherever the thumb has reached by then.
    /// Every figure the form carries is priced for a value the reader is passing through,
    /// which is what the panel beside the control is for.
    /// </summary>
    [Fact]
    public async Task ASweepIsAskedAboutWhileTheThumbIsDown()
    {
        var panel = await PanelAsync();
        var latency = Field(panel, Latency);

        latency.IsSweeping = true;
        latency.Slide = 2000;
        latency.Slide = 3000;
        latency.Slide = 4000;

        Assert.Equal(2, panel.Backend.Resolves);
        Assert.Equal(2000, panel.Backend.Draft(1).Viewer.SrtWatchLatencyMs);
        Assert.Equal("4000", latency.Readback);

        await panel.Backend.AnswerAsync(1);

        Assert.Equal(3, panel.Backend.Resolves);
        Assert.Equal(4000, panel.Backend.Draft(2).Viewer.SrtWatchLatencyMs);

        await panel.Backend.AnswerAsync(2);
        latency.IsSweeping = false;

        Assert.Equal(3, panel.Backend.Resolves);
        Assert.Equal(4000, (int)latency.Slide);
        Assert.Equal("4000", latency.Readback);
    }

    /// <summary>
    /// A repair is the one answer a sweep holds back: adopting it would walk the thumb
    /// out from under the pointer mid-gesture.
    /// It lands on the release, so the reader ends on the value the backend allows.
    /// </summary>
    [Fact]
    public async Task ARepairAnsweredDuringASweepLandsOnTheRelease()
    {
        var panel = await PanelAsync();
        var latency = Field(panel, Latency);
        panel.Backend.Repairs = settings => settings.Viewer.SrtWatchLatencyMs = 1000;

        latency.IsSweeping = true;
        latency.Slide = 4000;
        await panel.Backend.AnswerAsync(1);
        panel.Viewer.Apply();

        Assert.Equal(4000, (int)latency.Slide);

        latency.IsSweeping = false;
        panel.Viewer.Apply();

        Assert.Equal(1000, (int)latency.Slide);
    }

    /// <summary>
    /// What the reader is watching while they sweep: the price of the value under the thumb,
    /// which is the backend's answer and nothing this side derives (<c>docs/ipc-api.md</c>, "The rule").
    /// </summary>
    [Fact]
    public async Task TheCostFollowsTheThumb()
    {
        var backend = new DeferredBackend();
        var flow = Flows.Setup(backend);
        await backend.AnswerAsync(0);

        // Constant quality is the mode whose rate the quantizer prices.
        flow.Quality.Mode!.Options.Single(option => option.Value == "crf").Choose.Execute(null);
        await backend.AnswerAsync(1);

        var quantizer = flow.Quality.Quantizer!;
        var priced = flow.Rail.Bitrate;

        quantizer.IsSweeping = true;
        quantizer.Slide -= 6;
        await backend.AnswerAsync(2);

        Assert.NotEqual(priced, flow.Rail.Bitrate);
    }

    /// <summary>
    /// A render pass during a sweep draws the form it has, describing the value the sweep started from.
    /// Taking that value back into the control would put the thumb back under the pointer on every step.
    /// </summary>
    [Fact]
    public async Task ARenderDuringASweepLeavesTheThumbWhereTheReaderHasIt()
    {
        var panel = await PanelAsync();
        var latency = Field(panel, Latency);
        var started = latency.Slide;

        latency.IsSweeping = true;
        latency.Slide = 5000;

        panel.Viewer.Apply();
        panel.Viewer.Apply();

        Assert.NotEqual(started, latency.Slide);
        Assert.Equal(5000, (int)latency.Slide);

        // The write asked, and the two render passes over its unanswered form asked nothing.
        Assert.Equal(2, panel.Backend.Resolves);
    }

    /// <summary>
    /// The widget's half: the thumb's own two edges are what the reading is made of.
    /// Everything above rests on the binding carrying them,
    /// and a control watching the wrong events would leave every sweep looking like a settled value.
    /// </summary>
    [Fact]
    public void TheThumbsEdgesAreWhatTheControlReports()
    {
        var slider = new SweepSlider();

        Assert.False(slider.Sweeping);

        slider.RaiseEvent(new VectorEventArgs { RoutedEvent = Thumb.DragStartedEvent });
        Assert.True(slider.Sweeping);

        slider.RaiseEvent(new VectorEventArgs { RoutedEvent = Thumb.DragCompletedEvent });
        Assert.False(slider.Sweeping);
    }

    /// <summary>
    /// The figure is the control's own value and not the last answer's,
    /// so it is right in the window between a move and the form that confirms it.
    /// A keyboard step is that window at its shortest, a sweep that window held open.
    /// </summary>
    [Fact]
    public async Task TheFigureFollowsTheControlBeforeTheAnswerLands()
    {
        var panel = await PanelAsync();
        var latency = Field(panel, Latency);

        latency.Slide = 1500;

        Assert.Equal("1500", latency.Readback);
        Assert.Equal(2, panel.Backend.Resolves);

        await panel.Backend.AnswerAsync(1);

        Assert.Equal("1500", latency.Readback);
    }
}
