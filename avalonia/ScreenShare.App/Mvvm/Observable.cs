using System.ComponentModel;
using System.Runtime.CompilerServices;

namespace ScreenShare.App.Mvvm;

/// <summary>
/// Change notification a compiled binding reads.
/// Hand-rolled rather than taken from an MVVM toolkit: one method, no generated partials,
/// and no route by which a property is written outside the render function.
///
/// Raised on whichever thread wrote, and a binding tolerates the UI loop alone,
/// so an answer that arrived off it is marshalled before the write and not after.
/// </summary>
public abstract class Observable : INotifyPropertyChanged
{
    public event PropertyChangedEventHandler? PropertyChanged;

    /// <summary>
    /// Writes a field, notifying only where the value moved.
    /// What lets a render pass over unchanged input leave the window untouched.
    /// </summary>
    protected bool Set<T>(ref T field, T value, [CallerMemberName] string? property = null)
    {
        if (EqualityComparer<T>.Default.Equals(field, value))
        {
            return false;
        }

        field = value;
        PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(property));
        return true;
    }

    /// <summary>
    /// Notifies for a property with no field of its own, derived from ones <see cref="Set{T}"/> wrote.
    /// Raised by the render function after those writes: a computed property has nothing to compare,
    /// so nothing else tells a binding its inputs moved.
    /// </summary>
    protected void OnPropertyChanged(string property)
        => PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(property));
}
