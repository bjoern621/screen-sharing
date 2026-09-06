using Avalonia.Controls;
using Avalonia.Interactivity;
using Avalonia.Styling;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Insights.ViewModel;
using ScreenShare.App.Features.Shell.Go.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.Go.View;

/// <summary>
/// Markup, plus the menu markup cannot carry.
///
/// The rows follow the preset store, and a <c>MenuFlyout</c> realizes its items on the first open
/// (Avalonia 12.1.1, <c>Setup/AudioStep/View/AudioStepView.axaml.cs</c> states the failure),
/// so a fresh flyout is built at every press rather than one bound once and gone stale.
/// </summary>
public sealed partial class GoView : UserControl
{
    public GoView()
    {
        InitializeComponent();
    }

    private void OnMenuPressed(object? sender, RoutedEventArgs e)
    {
        var model = Assert.NotNull(DataContext as GoViewModel, "the menu button is bound to the strip commit");
        MenuOf(model).ShowAt(MenuButton);
    }

    /// <summary>
    /// The menu as the model stands at the press: commit with its summary, presets, stop while live,
    /// and the way into the wizard.
    /// </summary>
    private MenuFlyout MenuOf(GoViewModel model)
    {
        var flyout = new MenuFlyout { Placement = PlacementMode.BottomEdgeAlignedRight };

        flyout.Items.Add(new MenuItem { Header = model.CommitLabel, Command = model.CommitCommand });
        if (model.Summary.Length > 0)
        {
            flyout.Items.Add(new MenuItem { Theme = Note(), Header = model.Summary });
        }

        if (model.Builtin.Count + model.Saved.Count > 0)
        {
            flyout.Items.Add(new Separator());

            foreach (var row in model.Builtin)
            {
                flyout.Items.Add(PresetRowOf(row.Name, row.IsCurrent, row.IsReachable,
                    new DelegateCommand(() => model.UseBuiltin(row.Key))));
            }

            foreach (var row in model.Saved)
            {
                flyout.Items.Add(PresetRowOf(row.Name, row.IsCurrent, isReachable: true,
                    new DelegateCommand(() => model.UseSaved(row.Name))));
            }
        }

        if (model.IsLive)
        {
            flyout.Items.Add(new Separator());
            flyout.Items.Add(new MenuItem { Header = InsightsViewModel.StopLabel, Command = model.StopCommand });
        }

        flyout.Items.Add(new Separator());
        flyout.Items.Add(new MenuItem { Header = Strip.OpenSetup, Command = new DelegateCommand(model.OpenSetup) });

        return flyout;
    }

    /// <summary>
    /// One preset row: radio-marked where the draft delivers it, inert where nothing here reaches it.
    /// The reason stays on the rail card, a menu row having no line to carry it.
    /// </summary>
    private static MenuItem PresetRowOf(string name, bool isCurrent, bool isReachable, DelegateCommand press)
        => new()
        {
            Header = name,
            ToggleType = MenuItemToggleType.Radio,
            IsChecked = isCurrent,
            IsEnabled = isReachable,
            Command = press,
        };

    /// <summary>
    /// Off the application, where <c>Design/Menus.axaml</c> merges it:
    /// the view is asked before it is rooted, and an unrooted lookup walks no further than the control.
    /// </summary>
    private static ControlTheme Note()
    {
        var application = Assert.NotNull(Avalonia.Application.Current, "a view is rendered by a running application");
        return Assert.NotNull(application.FindResource("MenuNote") as ControlTheme,
            "the application's resources carry the inert menu row theme");
    }
}
