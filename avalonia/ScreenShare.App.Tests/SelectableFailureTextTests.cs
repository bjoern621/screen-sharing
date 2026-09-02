using System.Text.RegularExpressions;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A failure sentence is the one string that leaves the app for a bug report, so it is drawn with a control
/// a pointer can select (<c>docs/design-language.md</c>, "Status language").
/// Read off the markup: no view model reports which control drew its sentence.
/// </summary>
public sealed class SelectableFailureTextTests
{
    private static readonly Regex Element =
        new(@"<(?<tag>TextBlock|SelectableTextBlock)\b(?<attributes>[^>]*)>", RegexOptions.Compiled);

    /// <summary>The refusal role: why a control, an entry or a preset is inert (<c>Design/Text.axaml</c>).</summary>
    private static readonly Regex Refusal = new(@"Classes=""[^""]*\breason\b", RegexOptions.Compiled);

    /// <summary>A statement the backend made, under the name the view model binds it by.</summary>
    private static readonly Regex Statement =
        new(@"Text=""\{Binding [^""}]*(Reason|Refusal|Cause)\}""", RegexOptions.Compiled);

    [Fact]
    public void EveryRefusalSentenceIsDrawnWithASelectableControl()
    {
        var unselectable = new List<string>();

        foreach (var view in Views())
        {
            var markup = File.ReadAllText(view);
            foreach (Match element in Element.Matches(markup))
            {
                var attributes = element.Groups["attributes"].Value;
                if (element.Groups["tag"].Value != "TextBlock")
                {
                    continue;
                }
                if (!Refusal.IsMatch(attributes) && !Statement.IsMatch(attributes))
                {
                    continue;
                }

                var line = markup.Take(element.Index).Count(character => character == '\n') + 1;
                unselectable.Add($"{Path.GetFileName(view)}:{line}");
            }
        }

        Assert.True(unselectable.Count == 0,
            $"a failure sentence is a SelectableTextBlock: {string.Join(", ", unselectable)}");
    }

    /// <summary>Every view in the app, off the checkout the tests were built from.</summary>
    private static IEnumerable<string> Views()
    {
        var directory = new DirectoryInfo(AppContext.BaseDirectory);
        while (directory is not null && !File.Exists(Path.Combine(directory.FullName, "ScreenShare.App", "App.axaml")))
        {
            directory = directory.Parent;
        }

        Assert.NotNull(directory);
        var separator = Path.DirectorySeparatorChar;
        return Directory.EnumerateFiles(Path.Combine(directory.FullName, "ScreenShare.App"), "*.axaml",
                SearchOption.AllDirectories)
            .Where(path => !path.Contains($"{separator}obj{separator}") && !path.Contains($"{separator}bin{separator}"));
    }
}
