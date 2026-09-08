using System.Text.RegularExpressions;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A class a view names is defined by a style.
/// One that matches nothing leaves the control in the skin keyed on its type,
/// so a button meant to carry a picture keeps the 30-pixel height that skin sets and cuts the picture off.
/// Neither the build nor the run says a word about it, the class being a name and not a reference.
/// Read off the markup: no control reports which selectors reached it.
/// </summary>
public sealed class StyleClassTests
{
    private static readonly Regex Named = new(@"Classes=""(?<classes>[^""{]*)""", RegexOptions.Compiled);

    /// <summary>A class the view puts on a condition, <c>Classes.overbody="{Binding ...}"</c>.</summary>
    private static readonly Regex Bound = new(@"\bClasses\.(?<class>[A-Za-z0-9_]+)\s*=", RegexOptions.Compiled);

    private static readonly Regex Selector = new(@"Selector=""(?<selector>[^""]*)""", RegexOptions.Compiled);

    /// <summary>Class of a selector, whatever it is keyed on: <c>Border.tile</c>, <c>^.picked</c>, <c>.hint</c>.</summary>
    private static readonly Regex Matched = new(@"\.(?<class>[A-Za-z0-9_]+)", RegexOptions.Compiled);

    [Fact]
    public void EveryClassAViewNamesIsDefinedByAStyle()
    {
        var defined = new HashSet<string>(StringComparer.Ordinal);
        var named = new List<(string Class, string Where)>();

        foreach (var view in Markup.Views())
        {
            var markup = File.ReadAllText(view);

            foreach (Match selector in Selector.Matches(markup))
            {
                foreach (Match match in Matched.Matches(selector.Groups["selector"].Value))
                {
                    defined.Add(match.Groups["class"].Value);
                }
            }

            foreach (Match element in Named.Matches(markup))
            {
                var classes = element.Groups["classes"].Value
                    .Split(' ', StringSplitOptions.RemoveEmptyEntries);
                named.AddRange(classes.Select(name => (name, Where(view, markup, element.Index))));
            }

            foreach (Match element in Bound.Matches(markup))
            {
                named.Add((element.Groups["class"].Value, Where(view, markup, element.Index)));
            }
        }

        var orphans = named
            .Where(entry => !defined.Contains(entry.Class))
            .Select(entry => $"{entry.Where} names .{entry.Class}")
            .ToList();

        Assert.True(orphans.Count == 0,
            $"a class a view names is defined by a style: {string.Join(", ", orphans)}");
    }

    private static string Where(string view, string markup, int index)
    {
        var line = markup.Take(index).Count(character => character == '\n') + 1;
        return $"{Path.GetFileName(view)}:{line}";
    }
}
