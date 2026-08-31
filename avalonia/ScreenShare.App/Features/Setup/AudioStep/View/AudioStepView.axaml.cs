using System.ComponentModel;
using Avalonia.Controls;
using Avalonia.Controls.Primitives;
using Avalonia.Controls.Templates;
using Avalonia.Styling;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Setup.AudioStep.ViewModel;

namespace ScreenShare.App.Features.Setup.AudioStep.View;

/// <summary>
/// Markup, plus the one assignment markup cannot carry: the add button's flyout.
///
/// A <c>MenuFlyout</c> realizes its menu items from the entries it holds on its first open,
/// and an <c>ItemsSource</c> swapped while it is closed never reaches the realized items (Avalonia 12.1.1),
/// so a bound flyout keeps offering the first trailing row's entries
/// and every later pick writes <c>publish.audio_sources[0].source</c> instead of growing the list.
/// A fresh flyout per trailing row realizes that row's entries, so the write reaches the row past the end.
/// </summary>
public sealed partial class AudioStepView : UserControl
{
    private AudioStepViewModel? _model;

    public AudioStepView()
    {
        InitializeComponent();

        DataContextChanged += (_, _) => Watch();
    }

    /// <summary>Follows the view model the view is given, as the render function follows the group.</summary>
    private void Watch()
    {
        if (_model is not null)
        {
            _model.PropertyChanged -= OnModelChanged;
        }

        _model = DataContext as AudioStepViewModel;
        if (_model is not null)
        {
            _model.PropertyChanged += OnModelChanged;
        }

        ApplyAddMenu();
    }

    private void OnModelChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName is null or nameof(AudioStepViewModel.Add))
        {
            ApplyAddMenu();
        }
    }

    /// <summary>
    /// Safe to run twice: a flyout already listing the trailing row's entries is kept,
    /// so an unchanged pass replaces nothing and closes nothing.
    /// </summary>
    private void ApplyAddMenu()
    {
        var rows = _model?.Add?.MenuRows;
        if (rows is null)
        {
            AddButton.Flyout = null;
            return;
        }

        if (AddButton.Flyout is MenuFlyout standing)
        {
            if (ReferenceEquals(standing.ItemsSource, rows))
            {
                return;
            }

            standing.Hide();
        }

        // Off the application, where SelectEntry.axaml merges them:
        // this view is asked before it is rooted, and an unrooted lookup walks no further than the control.
        var application = Assert.NotNull(Avalonia.Application.Current, "a view is rendered by a running application");
        var entry = Assert.NotNull(application.FindResource("SelectEntry") as IDataTemplate,
            "the application's resources carry the dropdown row template");
        var item = Assert.NotNull(application.FindResource("SelectMenuItem") as ControlTheme,
            "the application's resources carry the dropdown item theme");

        AddButton.Flyout = new MenuFlyout
        {
            ItemsSource = rows,
            ItemTemplate = entry,
            ItemContainerTheme = item,
            Placement = PlacementMode.BottomEdgeAlignedLeft,
        };
    }
}
