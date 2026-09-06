using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Fields.Model;

/// <summary>
/// Which surface draws which group of the resolved form.
///
/// Placement is the shell's, and the contract describes no screens.
/// Which groups exist, in what order and with what under each is the backend's (<c>docs/ipc-api.md</c>, "The rule").
///
/// The watch group holds the legs a stream comes back on, the jitter buffers a receiver holds
/// and the chain a tile converts frames with.
/// None of it changes a byte this machine sends, so the viewer draws it beside the tiles it governs.
///
/// The app group holds what the app does for itself, which is about no stream at all,
/// so the dialog over the window draws it and a reader who never publishes still reaches it
/// (<see cref="Shell.Settings.ViewModel.AppSettingsViewModel"/>).
///
/// The wizard draws every other group.
/// Defaulting to it makes a group the backend adds a step that appears with nothing here to edit
/// (<see cref="Setup.Model.SetupSteps"/>).
/// </summary>
public static class GroupPlacement
{
    /// <summary>The receiving group as the form names it: the watch legs, the jitter buffers,
    /// the render chain.</summary>
    public const string WatchGroup = "watch";

    /// <summary>What the app does for itself: what it reports, and what it reads on start.</summary>
    public const string AppGroup = "app";

    /// <summary>Complement of the two placements below, so every group the form carries is drawn once.</summary>
    public static bool InSetup(string key)
    {
        Assert.That(key.Length > 0, "placing a group names the group being placed");

        return !InViewer(key) && !InAppSettings(key);
    }

    public static bool InViewer(string key)
    {
        Assert.That(key.Length > 0, "placing a group names the group being placed");

        return key == WatchGroup;
    }

    public static bool InAppSettings(string key)
    {
        Assert.That(key.Length > 0, "placing a group names the group being placed");

        return key == AppGroup;
    }
}
