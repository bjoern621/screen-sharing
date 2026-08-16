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
///
/// A sweep writes a value per pointer move, and each of them is a configuration the reader is passing through.
/// What the draft takes, what the screen prints and what the backend is asked are three different answers to
/// that, and this is where they are stated (<c>docs/settings-editing.md</c>).
/// </summary>
public sealed class SweepTests
{
    private static readonly Action<Action> Inline = action => action();

    private const string Latency = "viewer.srt_watch_latency_ms";

    private sealed record Panel(ViewerViewModel Viewer, FormSession Form, DeferredBackend Backend);

    /// <summary>The watch panel on an answered opening read, which is where the sliders are.</summary>
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
    /// The whole of the sweep: the draft follows the thumb, the figure beside it follows the draft, and the
    /// backend is asked once, about the value the reader stopped on.
    /// </summary>
    [Fact]
    public async Task ASweepAsksNothingUntilTheThumbIsLetGo()
    {
        var panel = await PanelAsync();
        var latency = Field(panel, Latency);

        latency.IsSweeping = true;
        latency.Slide = 2000;
        latency.Slide = 3000;
        latency.Slide = 4000;

        Assert.Equal(1, panel.Backend.Resolves);
        Assert.Equal("4000", latency.Readback);

        latency.IsSweeping = false;

        Assert.Equal(2, panel.Backend.Resolves);
        Assert.Equal(4000, panel.Backend.Draft(1).Viewer.SrtWatchLatencyMs);

        await panel.Backend.AnswerAsync(1);

        Assert.Equal(4000, (int)latency.Slide);
        Assert.Equal("4000", latency.Readback);
    }

    /// <summary>
    /// A render pass during a sweep draws the form it has, which describes the value the sweep started from.
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
        Assert.Equal(1, panel.Backend.Resolves);
    }

    /// <summary>
    /// The widget's half of it: the thumb's own two edges are what the reading is made of.
    /// Stated here because everything above rests on the binding carrying them, and a control that watched the
    /// wrong events would leave every sweep looking like a settled value.
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
    /// The figure is the control's own value and not the last answer's, so it is right in the window between a
    /// move and the form that confirms it.
    /// A keyboard step is that window at its shortest, and a sweep is that window held open.
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
