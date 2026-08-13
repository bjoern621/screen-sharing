using Avalonia.Controls.Primitives;

namespace ScreenShare.App.Controls;

/// <summary>
/// The wait: one arc turning at a constant rate.
/// It states no progress, because nothing here knows any - a control call is answered or it is not, and a
/// bar that filled itself would be this module inventing a figure the backend never sent
/// (<c>docs/ipc-api.md</c>).
/// Colour and size are inherited from whatever holds it, so a red commit, a plain button and a link restate
/// none of the design.
/// </summary>
public sealed class Spinner : TemplatedControl;
