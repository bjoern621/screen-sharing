using Avalonia.Controls.Primitives;

namespace ScreenShare.App.Controls;

/// <summary>
/// The wait: one arc turning at a constant rate, in the foreground of whatever holds it.
///
/// It states no progress, because nothing on this surface knows any - a control call is
/// answered or it is not, and a bar that filled itself would be this module inventing a
/// figure the backend never sent (<c>docs/ipc-api.md</c>).
///
/// It carries no colour and no size of its own either. Both are inherited from the control it
/// sits in, which is what lets one spinner appear on the red commit, on a plain button and on
/// a link without any of the three restating the design.
/// </summary>
public sealed class Spinner : TemplatedControl;
