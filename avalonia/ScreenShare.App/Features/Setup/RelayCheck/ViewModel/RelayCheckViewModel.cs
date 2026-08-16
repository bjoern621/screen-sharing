using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Setup.RelayCheck.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.RelayCheck.ViewModel;

/// <summary>
/// What answers on the relay the draft names, leg by leg.
///
/// Beside the address and the ports rather than anywhere else: those are the fields a wrong answer here is
/// corrected in, and a listener nothing reaches is otherwise met as a publish that waits out its connect window
/// and says "timeout".
///
/// Asked for and never taken on a render: dialling every leg costs seconds against a listener that is not
/// there, so a pass over the draft would put a relay under a keystroke's worth of connections.
/// What came back is held until the next press, being a reading of the moment the press was made rather than a
/// fact about the draft on screen now.
/// </summary>
public sealed class RelayCheckViewModel : Observable
{
    private readonly IBackend _backend;
    private readonly Action<Action> _dispatch;

    /// <summary>
    /// The draft to dial, read at the press rather than held: the reader may have typed another address since
    /// the last pass, and what is checked is the relay on screen.
    /// </summary>
    private readonly Func<Settings?> _draft;

    private readonly PendingCommand _check;

    /// <summary>What the last check answered, empty before one has run.</summary>
    private IReadOnlyList<RelayLegRow> _legs = [];

    /// <summary>
    /// What the backend said when the check itself could not be made, empty otherwise.
    /// A relay that answers nothing is not this: that comes back as rows saying so, so this is the call
    /// failing rather than the relay (<c>docs/ipc-api.md</c>, "Errors").
    /// </summary>
    private string _reason = "";

    /// <summary>Reader standing on the step this belongs to. Written by the flow.</summary>
    private bool _onStep;

    private bool _isVisible;
    private string _summary = "";
    private string _refusal = "";
    private bool _hasRefusal;
    private bool _hasLegs;

    public RelayCheckViewModel(IBackend backend, Func<Settings?> draft, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a relay check asks the backend to dial");
        Assert.NotNull(draft, "a relay check needs the draft naming the relay");
        Assert.NotNull(dispatch, "a relay check needs a UI loop to marshal an answer back to");

        _backend = backend;
        _draft = draft;
        _dispatch = dispatch;

        // Pressable once a relay is named: with no address there is nothing to dial, and the backend refuses a
        // draft that carries none.
        _check = new PendingCommand(CheckAsync, dispatch, () => _draft() is { Relay.Host.Length: > 0 });

        Legs = [];
        Render();
    }

    /// <summary>One line per leg, in the order the backend answered.</summary>
    public ObservableCollection<RelayLegRow> Legs { get; }

    public PendingCommand CheckCommand => _check;

    /// <summary>Drawn on the step holding the relay's own settings, and nowhere else.</summary>
    public bool IsVisible { get => _isVisible; private set => Set(ref _isVisible, value); }

    /// <summary>One line about the whole list, empty before a check has run.</summary>
    public string Summary { get => _summary; private set => Set(ref _summary, value); }

    /// <summary>Why the check could not be made, in the backend's own words.</summary>
    public string Refusal { get => _refusal; private set => Set(ref _refusal, value); }

    public bool HasRefusal { get => _hasRefusal; private set => Set(ref _hasRefusal, value); }

    /// <summary>Whether a check has answered, which is what the line about an unchecked relay stands in for.</summary>
    public bool HasLegs { get => _hasLegs; private set => Set(ref _hasLegs, value); }

    /// <summary>
    /// Which step the reader stands on, the flow's to say and never read off a widget.
    /// </summary>
    public void Apply(bool onStep)
    {
        _onStep = onStep;
        Render();
    }

    /// <summary>
    /// The whole component from the state above, every output written on every pass
    /// (<c>docs/development-principles.md</c>).
    /// </summary>
    private void Render()
    {
        IsVisible = _onStep;

        Reconcile.Onto(Legs, _legs);
        HasLegs = _legs.Count > 0;
        Summary = RelayLegRows.SummaryOf(_legs);

        Refusal = _reason;
        HasRefusal = _reason.Length > 0;

        _check.Refresh();
    }

    /// <summary>
    /// Dials every leg and draws what came back.
    ///
    /// What the last check said goes at the press rather than when this one answers: it is about a relay as it
    /// was, and leaving it up would put ticks from the old address beside a spinner about the new one.
    /// </summary>
    private async Task CheckAsync()
    {
        var draft = _draft();
        if (draft is null)
        {
            return;
        }

        _legs = [];
        _reason = "";
        Render();

        try
        {
            var legs = await _backend.CheckRelayAsync(draft).ConfigureAwait(false);
            _dispatch(() => Checked(legs));
        }
        catch (BackendUnavailableException e)
        {
            _dispatch(() => Failed(e.Message));
        }
        catch (OperationCanceledException)
        {
            // This call carries no token, so nothing cancels it.
            // A transport reporting one anyway still has to leave the button pressable rather than locked for
            // good.
            _dispatch(() => Failed(""));
        }
    }

    private void Checked(IReadOnlyList<RelayLeg> legs)
    {
        _legs = RelayLegRows.Of(legs);
        _reason = "";
        Render();
    }

    private void Failed(string reason)
    {
        _legs = [];
        _reason = reason;
        Render();
    }
}
