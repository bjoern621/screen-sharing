using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.Presets.ViewModel;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Saved ways of publishing: what the card lists, what a save sends, what applying one does to the draft,
/// and what a refused call leaves on screen.
/// Which presets exist and what is in each is the backend's, so the fixture is written through the call the
/// card makes and read back the same way.
/// The shell's half is what is asserted: rows follow the store, a save is followed by a read rather than by a
/// guess at what the store now holds, applying replaces the publish group alone, and the row in force is
/// derived from the settings rather than remembered from the press.
/// </summary>
public sealed class PresetsTests
{
    private static readonly Action<Action> Inline = action => action();

    private sealed record Card(
        PresetsViewModel Presets, FormSession Form, SeededBackend Backend, SetupViewModel Flow);

    /// <summary>
    /// A flow whose first form has landed and whose preset card has read the store once.
    /// Answers come from memory over an inline dispatcher, so what a test reads next is what the render pass
    /// wrote.
    /// </summary>
    private static async Task<Card> CardAsync(SeededBackend? seeded = null)
    {
        var backend = seeded ?? new SeededBackend("linux");
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        var flow = new SetupViewModel(backend, form, session, Inline);

        await form.Settled;
        await flow.Rail.Presets.Settled;

        return new Card(flow.Rail.Presets, form, backend, flow);
    }

    /// <summary>The draft's way of publishing, what a save carries.</summary>
    private static PublishSettings Publish(Card card) => card.Form.Draft!.Publish;

    private static PresetRow Row(Card card, string name)
        => Assert.Single(card.Presets.Rows, row => row.Name == name);

    private static BuiltinPresetRow Builtin(Card card, string key)
        => Assert.Single(card.Presets.Builtin, row => row.Key == key);

    /// <summary>One field write, on the path a control the reader moved takes.</summary>
    private static void Write(Card card, string key, string value)
        => card.Form.Write(key, new FieldValue { Text = value });

    /// <summary>
    /// Nothing on the contract announces a preset, so a card that waited to be told would show an empty list
    /// to a reader who has saved twenty.
    /// Order is the store's.
    /// </summary>
    [Fact]
    public async Task TheCardListsWhatTheStoreHeldWhenItOpened()
    {
        var backend = new SeededBackend("linux");
        await backend.SavePresetAsync("work", new PublishSettings { Name = "work" });
        await backend.SavePresetAsync("lan", new PublishSettings { Name = "lan" });

        var card = await CardAsync(backend);

        Assert.Equal(["work", "lan"], card.Presets.Rows.Select(row => row.Name));
        Assert.False(card.Presets.IsEmpty);
        Assert.Equal("", card.Presets.Refusal);
    }

    /// <summary>A store that answered and holds nothing is a state, not a failure, so it gets its own sentence.</summary>
    [Fact]
    public async Task AnEmptyStoreSaysNothingIsSavedYet()
    {
        var card = await CardAsync();

        Assert.Empty(card.Presets.Rows);
        Assert.True(card.Presets.IsEmpty);
        Assert.False(card.Presets.HasNotice);
    }

    /// <summary>The row that appears comes from reading the store again, not from what was sent.</summary>
    [Fact]
    public async Task SavingKeepsTheDraftUnderTheTypedName()
    {
        var card = await CardAsync();

        card.Presets.Name = "work";
        card.Presets.SaveCommand.Execute(null);
        await card.Presets.Settled;

        var row = Row(card, "work");
        Assert.True(row.IsCurrent);

        var stored = Assert.Single((await card.Backend.PresetsAsync()).Saved);
        Assert.Equal("work", stored.Name);
        Assert.Equal(Publish(card), stored.Settings);
    }

    /// <summary>
    /// The store keys on the name, so saving over a preset is how one is edited.
    /// The word on the button is the only warning before the press.
    /// </summary>
    [Fact]
    public async Task ANameAlreadyInTheStoreReplacesRatherThanAddingASecondRow()
    {
        var card = await CardAsync();

        card.Presets.Name = "work";
        Assert.Equal("Save", card.Presets.SaveLabel);

        card.Presets.SaveCommand.Execute(null);
        await card.Presets.Settled;

        Write(card, "publish.name", "moved");
        card.Presets.Name = "work";
        Assert.Equal("Replace", card.Presets.SaveLabel);

        card.Presets.SaveCommand.Execute(null);
        await card.Presets.Settled;

        var row = Assert.Single(card.Presets.Rows);
        Assert.Equal("work", row.Name);

        var stored = Assert.Single((await card.Backend.PresetsAsync()).Saved);
        Assert.Equal("moved", stored.Settings.Name);
    }

    /// <summary>Text left in the box would offer to replace the preset that was just written.</summary>
    [Fact]
    public async Task TheNameBoxIsEmptyOnceWhatWasInItIsSaved()
    {
        var card = await CardAsync();

        card.Presets.Name = "work";
        card.Presets.SaveCommand.Execute(null);
        await card.Presets.Settled;

        Assert.Equal("", card.Presets.Name);
        Assert.False(card.Presets.CanSave);
    }

    /// <summary>
    /// Spaces are an empty name, and the backend refuses one, so sending it would be asking for a refusal the
    /// card could answer itself.
    /// </summary>
    [Fact]
    public async Task ANameWithNothingInItIsNotOfferedAsASave()
    {
        var card = await CardAsync();

        Assert.False(card.Presets.CanSave);
        Assert.False(card.Presets.SaveCommand.CanExecute(null));

        card.Presets.Name = "   ";

        Assert.False(card.Presets.CanSave);
        Assert.False(card.Presets.SaveCommand.CanExecute(null));
        Assert.Empty((await card.Backend.PresetsAsync()).Saved);
    }

    /// <summary>The row goes because the store is read again, not because the list was patched.</summary>
    [Fact]
    public async Task DeletingTakesTheRowOff()
    {
        var backend = new SeededBackend("linux");
        await backend.SavePresetAsync("work", new PublishSettings { Name = "work" });
        await backend.SavePresetAsync("lan", new PublishSettings { Name = "lan" });

        var card = await CardAsync(backend);

        Row(card, "work").Delete.Execute(null);
        await card.Presets.Settled;

        Assert.Equal(["lan"], card.Presets.Rows.Select(row => row.Name));
        Assert.Equal("", card.Presets.Refusal);
    }

    /// <summary>
    /// The relay belongs to a deployment and the watch settings to this machine, so a preset that carried
    /// either would break where it was meant to help (<c>docs/presets.md</c>).
    /// </summary>
    [Fact]
    public async Task ApplyingReplacesThePublishGroupAndLeavesTheOthersAlone()
    {
        var card = await CardAsync();

        // Saved off the draft with one field moved, so the repair returns the preset itself rather than a
        // walked-to-legal version of it.
        var kept = Publish(card).Clone();
        kept.Name = "from-preset";
        await card.Backend.SavePresetAsync("work", kept);

        card.Presets.RereadCommand.Execute(null);
        await card.Presets.Settled;

        Write(card, "relay.host", "relay.example");
        Write(card, "publish.name", "typed-since");
        await card.Form.Settled;

        Row(card, "work").Apply.Execute(null);
        await card.Form.Settled;

        Assert.Equal("from-preset", card.Form.Draft!.Publish.Name);
        Assert.Equal("relay.example", card.Form.Draft.Relay.Host);
    }

    /// <summary>
    /// A snapshot claims nothing about a region of the settings space, so being in force can only mean being
    /// equal.
    /// No stored selection is left to disagree with the draft (<c>docs/presets.md</c>, "Saved presets").
    /// </summary>
    [Fact]
    public async Task ThePresetTheDraftEqualsIsMarkedAndAnEditUnmarksIt()
    {
        var card = await CardAsync();

        card.Presets.Name = "work";
        card.Presets.SaveCommand.Execute(null);
        await card.Presets.Settled;

        Assert.True(Row(card, "work").IsCurrent);

        Write(card, "publish.name", "moved");
        await card.Form.Settled;

        Assert.False(Row(card, "work").IsCurrent);

        Row(card, "work").Apply.Execute(null);
        await card.Form.Settled;

        Assert.True(Row(card, "work").IsCurrent);
    }

    /// <summary>A way of publishing is staged until a commit carries it, so trying a preset out is free.</summary>
    [Fact]
    public async Task ApplyingCommitsNothing()
    {
        var card = await CardAsync();

        card.Presets.Name = "work";
        card.Presets.SaveCommand.Execute(null);
        await card.Presets.Settled;

        Row(card, "work").Apply.Execute(null);
        await card.Form.Settled;

        Assert.Empty(card.Backend.Started);
        Assert.Empty(card.Backend.Saved);
    }

    /// <summary>
    /// The refusal is the backend's sentence, shown as it stands.
    /// The rows are still the store's last answer, and the reader may yet fix what the sentence named.
    /// </summary>
    [Fact]
    public async Task ARefusedSaveSaysWhyAndKeepsTheList()
    {
        var backend = new SeededBackend("linux");
        await backend.SavePresetAsync("work", new PublishSettings { Name = "work" });

        var card = await CardAsync(backend);
        backend.PresetRefusal = "cannot save the preset 'lan': the presets file is read-only";

        card.Presets.Name = "lan";
        card.Presets.SaveCommand.Execute(null);
        await card.Presets.Settled;

        Assert.Equal(backend.PresetRefusal, card.Presets.Refusal);
        Assert.True(card.Presets.HasRefusal);
        Assert.Equal(["work"], card.Presets.Rows.Select(row => row.Name));

        // The save it was typed for did not happen, so the name stays in the box.
        Assert.Equal("lan", card.Presets.Name);
    }

    /// <summary>
    /// Re-reading off the failure would replace a sentence nobody has read yet, so the re-read is a press.
    /// A preset another window deleted first is the case that produces it.
    /// </summary>
    [Fact]
    public async Task ADeleteTheStoreRefusesLeavesTheRowAndTheReasonUp()
    {
        var backend = new SeededBackend("linux");
        await backend.SavePresetAsync("work", new PublishSettings { Name = "work" });

        var card = await CardAsync(backend);
        await backend.DeletePresetAsync("work");

        Row(card, "work").Delete.Execute(null);
        await card.Presets.Settled;

        Assert.Equal("no preset named 'work'", card.Presets.Refusal);
        Assert.Equal(["work"], card.Presets.Rows.Select(row => row.Name));

        card.Presets.RereadCommand.Execute(null);
        await card.Presets.Settled;

        Assert.Empty(card.Presets.Rows);
        Assert.Equal("", card.Presets.Refusal);
    }

    /// <summary>
    /// Nothing saved and nothing readable are different facts, and only the notice names where the old file
    /// went.
    /// </summary>
    [Fact]
    public async Task AnUnreadableStoreCarriesTheNoticeRatherThanTheEmptySentence()
    {
        var backend = new SeededBackend("linux")
        {
            PresetNotice = new Text
            {
                Code = TextCode.PresetStoreUnreadable,
                Args = { new TextArg { Name = TextArgName.Path, Id = "/tmp/presets.json.broken" } },
            },
        };

        var card = await CardAsync(backend);

        Assert.Empty(card.Presets.Rows);
        Assert.False(card.Presets.IsEmpty);
        Assert.True(card.Presets.HasNotice);
        Assert.Contains("/tmp/presets.json.broken", card.Presets.Notice);
    }

    /// <summary>Rows that compare equal are what let the review render on every keystroke.</summary>
    [Fact]
    public async Task RenderingTwiceOverOneStoreLeavesTheRowsAlone()
    {
        var backend = new SeededBackend("linux");
        await backend.SavePresetAsync("work", new PublishSettings { Name = "work" });

        var card = await CardAsync(backend);
        var before = card.Presets.Rows.ToList();
        var beforeBuiltin = card.Presets.Builtin.ToList();

        card.Presets.Apply();
        card.Flow.Apply();

        Assert.Equal(before, card.Presets.Rows);
        Assert.Equal(beforeBuiltin, card.Presets.Builtin);
    }

    // --- The built-in presets ---------------------------------------------------------
    //
    // The other kind of preset.
    // A saved one is a snapshot under a name; a built-in one is a promise about the picture, resolved against
    // this machine by the backend (docs/presets.md).

    /// <summary>
    /// Which presets exist is not a shell's to know, so the rows are the form's, each carrying the words this
    /// shell keeps for the identifier.
    /// </summary>
    [Fact]
    public async Task TheCardListsTheBuiltInPresetsTheFormCarried()
    {
        var card = await CardAsync();

        Assert.Equal(["lossless", "gaming", "readability"], card.Presets.Builtin.Select(row => row.Key));
        Assert.Equal("Gaming", Builtin(card, "gaming").Name);
    }

    [Fact]
    public async Task ApplyingABuiltInPresetWritesWhatTheBackendResolvedForThisMachine()
    {
        var card = await CardAsync();

        Write(card, "relay.host", "relay.example");
        await card.Form.Settled;

        var resolved = Builtin(card, "gaming");
        Assert.True(resolved.IsReachable);

        // The relay is an applied group, so the write above persisted on its own.
        // Counted here is what the apply adds to that, which is nothing.
        var savedBefore = card.Backend.Saved.Count;

        resolved.Apply.Execute(null);
        await card.Form.Settled;

        Assert.Equal("cbr", Publish(card).Mode);
        Assert.Equal(60, Publish(card).Fps);
        Assert.Equal("relay.example", card.Form.Draft!.Relay.Host);
        Assert.Empty(card.Backend.Started);
        Assert.Equal(savedBefore, card.Backend.Saved.Count);
    }

    /// <summary>
    /// The mark is derived from the settings on every resolve rather than remembered from the press.
    /// The promises are written so that no settings deliver two of them.
    /// </summary>
    [Fact]
    public async Task ApplyingABuiltInPresetMarksItAndNothingElse()
    {
        var card = await CardAsync();

        Builtin(card, "gaming").Apply.Execute(null);
        await card.Form.Settled;

        Assert.True(Builtin(card, "gaming").IsCurrent);
        Assert.Single(card.Presets.Builtin, row => row.IsCurrent);
    }

    /// <summary>
    /// A promise covers a region rather than a snapshot: a field it says nothing about moves without taking
    /// the mark off, and one it fixes takes it off.
    /// A saved preset is marked only while the draft equals it field for field.
    /// </summary>
    [Fact]
    public async Task AnEditInsideThePromiseKeepsTheBuiltInPresetMarked()
    {
        var card = await CardAsync();

        Builtin(card, "gaming").Apply.Execute(null);
        await card.Form.Settled;

        card.Form.Write("publish.bitrate_mbps", new FieldValue { Number = 80 });
        await card.Form.Settled;

        Assert.True(Builtin(card, "gaming").IsCurrent);

        card.Form.Write("publish.fps", new FieldValue { Number = 30 });
        await card.Form.Settled;

        Assert.False(Builtin(card, "gaming").IsCurrent);
    }

    /// <summary>
    /// An unreachable preset keeps its row, greyed, under the backend's reason, which is the treatment every
    /// ruled-out choice gets.
    /// The press does nothing: a repaired near miss would be a way of publishing the reader did not ask for,
    /// under the name of one they did.
    /// </summary>
    [Fact]
    public async Task APresetNothingHereReachesKeepsItsRowAndSaysWhy()
    {
        var card = await CardAsync();

        // An encoder that takes no planar RGB, what the lossless promise rests on.
        Write(card, "publish.codec", "libx264");
        await card.Form.Settled;

        var lossless = Builtin(card, "lossless");
        Assert.False(lossless.IsReachable);
        Assert.False(lossless.Apply.CanExecute(null));
        Assert.Contains("Lossless", lossless.Reason);
        Assert.Contains("SRT", lossless.Reason);

        var before = Publish(card).Clone();
        lossless.Apply.Execute(null);
        await card.Form.Settled;

        Assert.Equal(before, Publish(card));
    }
}
