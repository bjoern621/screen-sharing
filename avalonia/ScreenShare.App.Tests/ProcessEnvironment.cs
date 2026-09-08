using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Test classes writing the process environment, run one at a time.
/// xunit runs a class's tests in sequence and separate classes at once,
/// and a variable is the process's, so two such classes at once each read the other's value.
/// Which classes are in it decides nothing on its own: what a class here writes, it writes for the whole run.
/// </summary>
[CollectionDefinition(Name)]
public sealed class ProcessEnvironment
{
    public const string Name = "process environment";
}
