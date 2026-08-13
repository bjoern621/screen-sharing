using System.Diagnostics.CodeAnalysis;

namespace ScreenShare.App.Contracts;

/// <summary>
/// Thrown when an internal contract stops holding: an Entwicklungsfehler, never a condition to survive.
/// Nothing catches it, so the process ends at the frame that broke the contract, carrying the stack that led
/// there.
/// An Umgebungsfehler, an unreachable relay or a malformed response, travels as a value for a screen to show.
/// </summary>
public sealed class ContractViolationException(string message) : Exception(message);

/// <summary>
/// C# counterpart of <c>bjoernblessin.de/go-utils/util/assert</c>, which the Go modules state their contracts
/// with.
///
/// Not <see cref="System.Diagnostics.Debug.Assert(bool)"/>: that is compiled out of a Release build, and a
/// contract holding in Debug alone is no contract.
/// These run and throw in every build.
///
/// A message is a present-tense sentence naming the invariant that holds, never the failure that occurred:
/// lowercase, no trailing period, offending values in the trailing arguments (docs/development-principles.md).
/// </summary>
public static class Assert
{
    public static void That([DoesNotReturnIf(false)] bool condition, string invariant, params object?[] values)
    {
        if (condition)
        {
            return;
        }

        throw new ContractViolationException(Sentence(invariant, values));
    }

    /// <summary>Asserts a reference the caller is about to dereference.</summary>
    public static T NotNull<T>([NotNull] T? value, string invariant) where T : class
    {
        if (value is null)
        {
            throw new ContractViolationException(Sentence(invariant, []));
        }

        return value;
    }

    /// <summary>
    /// Ends an exhaustive dispatch.
    /// The one inversion of the message style: names what turned up, since no invariant is left to state.
    /// </summary>
    [DoesNotReturn]
    public static void Never(string what, params object?[] values)
        => throw new ContractViolationException(Sentence(what, values));

    /// <summary>
    /// <see cref="Never(string, object?[])"/> in expression position, for the default arm of a switch that
    /// has to produce a value.
    /// Produces none.
    /// </summary>
    [DoesNotReturn]
    public static T Never<T>(string what, params object?[] values)
        => throw new ContractViolationException(Sentence(what, values));

    private static string Sentence(string sentence, object?[] values)
        => values.Length == 0
            ? sentence
            : $"{sentence} ({string.Join(", ", values.Select(value => value?.ToString() ?? "<null>"))})";
}
