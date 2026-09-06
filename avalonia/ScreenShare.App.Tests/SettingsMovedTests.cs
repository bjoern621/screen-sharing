using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The backend's settings move for reasons no window caused: a Discord link lands, another window saves,
/// a start writes the draft it was handed.
/// The announcement is how a shell hears of it (<c>docs/ipc-api.md</c>), and the draft a window holds is a
/// copy, so hearing it means reading the settings again rather than saving over what this window never saw.
/// </summary>
public sealed class SettingsMovedTests
{
    private static readonly Action<Action> Inline = action => action();

    private sealed record Window(FormSession Form, Session Session, SeededBackend Backend);

    /// <summary>A window that has read once, over a backend with one announcement waiting.</summary>
    private static async Task<Window> WindowAsync()
    {
        var backend = new SeededBackend("linux");
        backend.Announcements.Add(new Event { SettingsChanged = new SettingsChanged() });
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);

        await form.Settled;
        return new Window(form, session, backend);
    }

    /// <summary>Runs the session, which is what delivers the announcement waiting on the stream.</summary>
    private static async Task AnnounceAsync(Window window)
    {
        _ = window.Session.Start();
        while (!window.Session.IsLoaded)
        {
            await Task.Delay(1);
        }
        await window.Form.Settled;
    }

    /// <summary>Settings the backend holds after a write this window did not make.</summary>
    private static Settings Moved(Settings from, string host)
    {
        var moved = from.Clone();
        moved.Relay.Host = host;
        return moved;
    }

    /// <summary>
    /// A draft nobody has edited is the backend's settings, so it becomes what they became.
    /// </summary>
    [Fact]
    public async Task AnAnnouncedWriteLandsInADraftNobodyIsEditing()
    {
        var window = await WindowAsync();
        window.Backend.Stored = Moved(window.Form.Draft!, "moved.example");

        await AnnounceAsync(window);

        Assert.Equal("moved.example", window.Form.Draft!.Relay.Host);
    }

    /// <summary>
    /// A staged edit is uncommitted work, so no announcement takes it off the screen.
    /// The backend's own copy still moves under it, which is the difference the two of them exist to hold
    /// (<c>docs/settings-editing.md</c>, "Staged and applied").
    /// </summary>
    [Fact]
    public async Task AnAnnouncedWriteLeavesUncommittedEditsStanding()
    {
        var window = await WindowAsync();
        window.Backend.Stored = Moved(window.Form.Draft!, "moved.example");
        window.Form.Write("publish.fps", new FieldValue { Number = 120 });

        await AnnounceAsync(window);

        Assert.Equal(120, window.Form.Draft!.Publish.Fps);
        Assert.Equal("moved.example", window.Form.Stored!.Relay.Host);
    }
}
