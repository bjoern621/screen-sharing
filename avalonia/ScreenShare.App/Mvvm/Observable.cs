using System.ComponentModel;
using System.Runtime.CompilerServices;

namespace ScreenShare.App.Mvvm;

/// <summary>
/// The change notification a compiled binding reads.
/// Hand-rolled rather than pulled from an MVVM toolkit: one method, no generated partials, and nothing that
/// would let a property be written from somewhere other than the render function.
/// </summary>
public abstract class Observable : INotifyPropertyChanged
{
    public event PropertyChangedEventHandler? PropertyChanged;

    /// <summary>
    /// Writes a field and notifies only when the value actually differs, which is what makes a render pass
    /// with unchanged input produce no visible change.
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
    /// Notifies for a property that has no field of its own: one derived from the fields <see cref="Set{T}"/>
    /// has just written.
    /// The render function raises these itself, after the writes they read, because a binding on a computed
    /// property has nothing to compare and would otherwise never hear that its inputs moved.
    /// </summary>
    protected void OnPropertyChanged(string property)
        => PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(property));
}
