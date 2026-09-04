using ScreenShare.App.Contracts;
using ScreenShare.App.Mvvm;
using TablerIcons;

namespace ScreenShare.App.Features.Fields.ViewModel;

/// <summary>
/// Disclosure over a step's folded controls.
///
/// Closed on every launch, and the reader's press is the one opener:
/// the fold is view state the reader set, like an opened refused list
/// (<see cref="FieldViewModel.RefusedShown"/>), so a render pass keeps it and nothing persists it.
/// The rows behind it stay drawn; opening is visibility alone.
/// </summary>
public sealed class FoldViewModel : Observable
{
    private bool _shown;
    private bool _hasAny;
    private string _count = "";

    public FoldViewModel()
    {
        ToggleCommand = new DelegateCommand(() => Shown = !Shown);
    }

    /// <summary>Flips the fold. Two presses are two states, the one deliberate departure from idempotency.</summary>
    public DelegateCommand ToggleCommand { get; }

    /// <summary>Reader-owned: written by the press alone.</summary>
    public bool Shown
    {
        get => _shown;
        set
        {
            if (Set(ref _shown, value))
            {
                OnPropertyChanged(nameof(Glyph));
            }
        }
    }

    /// <summary>Whether the step holds anything to fold. The disclosure row draws only while it does.</summary>
    public bool HasAny { get => _hasAny; private set => Set(ref _hasAny, value); }

    /// <summary>Controls behind the fold, beside the disclosure: the figure says whether opening it is worth the trip.</summary>
    public string Count { get => _count; private set => Set(ref _count, value); }

    public Icons Glyph => Shown ? Icons.IconChevronDown : Icons.IconChevronRight;

    /// <summary>Render-pass input. Idempotent, and it never writes <see cref="Shown"/>.</summary>
    public void Apply(int folded)
    {
        Assert.That(folded >= 0, "a fold counts the controls behind it", folded);

        HasAny = folded > 0;
        Count = folded > 0 ? Copy.Fields.OptionCount(folded) : "";

        Assert.That(HasAny == (Count.Length > 0), "the disclosure and its figure agree", HasAny, Count);
    }
}
