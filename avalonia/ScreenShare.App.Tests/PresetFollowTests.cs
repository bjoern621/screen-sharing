using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The detach rule: a write to a field the followed preset decides clears the followed key
/// with the edit, and a write anywhere else keeps the draft following.
///
/// Which fields those are is the form's answer (<c>form.proto</c>, Form.preset_owned_field_keys),
/// so what is asserted here is what the shell does with that answer, as the applied tests do
/// with FieldGroup.applied.
/// </summary>
public sealed class PresetFollowTests
{
    private static readonly Action<Action> Inline = action => action();

    private static async Task<FormSession> FollowingAsync()
    {
        var backend = new SeededBackend("linux");
        var form = new FormSession(backend, new Session(backend, Inline), Inline);
        await form.Settled;

        var publish = form.Draft!.Publish.Clone();
        publish.Preset = "balanced";
        form.WritePublish(publish);
        return form;
    }

    /// <summary>The user took a value the preset decides into their own hands.</summary>
    [Fact]
    public async Task AWriteToAPresetOwnedFieldDetaches()
    {
        var form = await FollowingAsync();

        form.Write("publish.fps", new FieldValue { Number = 30 });

        Assert.Equal("", form.Draft!.Publish.Preset);
        Assert.Equal(30, form.Draft!.Publish.Fps);
    }

    /// <summary>
    /// The uplink is an input the preset prices from rather than a value it decides,
    /// so stating the line keeps the promise in force.
    /// </summary>
    [Fact]
    public async Task AWriteElsewhereKeepsTheDraftFollowing()
    {
        var form = await FollowingAsync();

        form.Write("publish.uplink_mbps", new FieldValue { Number = 25 });

        Assert.Equal("balanced", form.Draft!.Publish.Preset);
        Assert.Equal(25, form.Draft!.Publish.UplinkMbps);
    }
}
