using System.Diagnostics.CodeAnalysis;

namespace ScreenShare.App.Contracts;

/// <summary>
/// Raised when an internal contract stops holding.
/// This is an Entwicklungsfehler: a bug in this code, never a condition the app is expected to survive.
/// An environment failure - an unreachable relay, a malformed response - is carried as a value instead, so
/// the UI can show it.
/// </summary>
public sealed class ContractViolationException(string message) : Exception(message);

/// <summary>
/// The C# counterpart of <c>bjoernblessin.de/go-utils/util/assert</c>, which both Go modules state their
/// contracts with.
///
/// Deliberately not <see cref="System.Diagnostics.Debug.Assert(bool)"/>: that one is compiled out of a
/// Release build, and a contract that only holds in Debug is not a contract.
/// These always run and always throw.
///
/// An assertion message is a present-tense sentence naming the invariant that holds, not the failure that
/// occurred.
/// Lowercase, no trailing period, offending values in the trailing arguments rather than in the sentence
/// (docs/development-principles.md).
/// </summary>
public static class Assert
{
    /// <summary>Asserts an invariant. The message states the world when the code is correct.</summary>
    public static void That([DoesNotReturnIf(false)] bool condition, string invariant, params object?[] values)
    {
        if (condition)
        {
            return;
        }

        throw new ContractViolationException(Sentence(invariant, values));
    }

    /// <summary>Asserts a reference the caller is about to dereference is present, and returns it.</summary>
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
    /// The one inversion of the message style: it names what turned up instead of the invariant, because
    /// there is no invariant left.
    /// </summary>
    [DoesNotReturn]
    public static void Never(string what, params object?[] values)
        => throw new ContractViolationException(Sentence(what, values));

    /// <summary>
    /// <see cref="Never(string, object?[])"/> in expression position, for the default arm of a switch that
    /// has to produce a value.
    /// It never returns one.
    /// </summary>
    [DoesNotReturn]
    public static T Never<T>(string what, params object?[] values)
        => throw new ContractViolationException(Sentence(what, values));

    private static string Sentence(string sentence, object?[] values)
        => values.Length == 0
            ? sentence
            : $"{sentence} ({string.Join(", ", values.Select(value => value?.ToString() ?? "<null>"))})";
}
