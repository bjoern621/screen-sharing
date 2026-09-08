using System.ComponentModel;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Threading;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Shell.Settings.Model;
using ScreenShare.App.Features.Shell.Settings.ViewModel;

namespace ScreenShare.App.Features.Shell.Settings.View;

/// <summary>
/// Three things the markup cannot state: two dismissals, and the heading the dialog opens at.
/// Every setting the dialog draws writes through its own binding.
/// </summary>
public sealed partial class AppSettingsView : UserControl
{
    /// <summary>Dialog behind this view, held so its news is dropped when the binding moves.</summary>
    private AppSettingsViewModel? _settings;

    public AppSettingsView()
    {
        InitializeComponent();

        // Escape is bound here and on the window, which uses it to leave a filled viewer.
        // A key event starts at the focus and travels outward, so the focus moving here as the dialog opens
        // is what puts this binding first.
        PropertyChanged += (_, change) =>
        {
            if (change.Property == IsVisibleProperty && IsVisible)
            {
                Focus();
            }
        };

        DataContextChanged += (_, _) => Follow(DataContext as AppSettingsViewModel);
    }

    /// <summary>
    /// Takes the dialog's news, dropping the last one's.
    /// The scroll follows the model rather than this control's own visibility:
    /// the scrim carries whether the dialog is drawn, and this control is in the tree either way.
    /// </summary>
    private void Follow(AppSettingsViewModel? settings)
    {
        if (_settings is not null)
        {
            _settings.PropertyChanged -= Moved;
        }

        _settings = settings;

        if (_settings is not null)
        {
            _settings.PropertyChanged += Moved;
        }
    }

    private void Moved(object? sender, PropertyChangedEventArgs change)
    {
        if (_settings is not { IsOpen: true })
        {
            return;
        }

        if (change.PropertyName is nameof(AppSettingsViewModel.IsOpen)
            or nameof(AppSettingsViewModel.OpenedAt))
        {
            StandAtOpenedSection(_settings.OpenedAt);
        }
    }

    /// <summary>
    /// Puts the heading the press asked for in front of the reader.
    /// The offset carries between openings, so a plain open scrolls home rather than standing where
    /// the opening before it left off.
    /// Posted at <see cref="DispatcherPriority.Loaded"/>: a panel the layout has not measured has nowhere
    /// to be scrolled to.
    /// </summary>
    private void StandAtOpenedSection(SettingsSection section)
    {
        Dispatcher.UIThread.Post(
            () =>
            {
                switch (section)
                {
                    case SettingsSection.Top:
                        Sections.ScrollToHome();
                        break;

                    case SettingsSection.Discord:
                        DiscordSection.BringIntoView();
                        break;

                    default:
                        Assert.Never("unexpected settings section", (int)section);
                        break;
                }
            },
            DispatcherPriority.Loaded);
    }

    /// <summary>
    /// Press on the dimmed ground, which closes.
    /// The press has to have landed on the ground itself:
    /// one inside the dialog reaches this handler on its way out,
    /// and closing on it would dismiss the dialog whenever anything in it was clicked.
    /// </summary>
    private void ScrimPressed(object? sender, PointerPressedEventArgs e)
    {
        if (ReferenceEquals(e.Source, sender) && DataContext is AppSettingsViewModel settings)
        {
            settings.CloseCommand.Execute(null);
        }
    }
}
