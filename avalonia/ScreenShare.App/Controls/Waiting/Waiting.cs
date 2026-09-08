using Avalonia.Controls.Primitives;

namespace ScreenShare.App.Controls;

/// <summary>
/// Wait a screen is in rather than one a control was pressed into: the arc stands beside the sentence
/// saying what is outstanding, and replaces nothing.
///
/// Split from <see cref="Pending"/> by who asked.
/// A press is answered in the box the label had, so that arc is attached to the control and takes it over.
/// A read nobody pressed for has no box to take over, so this one is placed.
///
/// One definition rather than the arc, its hue and its pointer target restated per screen:
/// a bare <see cref="Spinner"/> inherits neither, and a view stating the hue would be a colour outside
/// <c>Design/</c> (<c>avalonia/README.md</c>).
/// A screen states only whether it is waiting, and a tip where the sentence beside it does not already say why.
/// </summary>
public sealed class Waiting : TemplatedControl;
