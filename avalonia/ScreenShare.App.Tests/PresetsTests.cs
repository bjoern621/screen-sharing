using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.Presets.ViewModel;
using ScreenShare.App.Features.Setup.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The saved ways of publishing: what the card lists, what a save sends, what applying one does to the draft,
/// and what it says when a call is refused.
///
/// <b>Everything here is the store's answer, and these tests state what the card did with it.</b> Which
/// presets exist and what is inside each one are the backend's, so the fixture is written to by the same call
/// the card makes and read back the same way.
/// What is checked is the shell's half: the rows follow the store, a save is followed by a read rather than
/// by a guess about what the store now holds, applying replaces the publish group and nothing else, and the
/// row marked as the one in force is derived from the settings rather than remembered from the press.
/// </summary>
public sealed class PresetsTests
{
    private static readonly Action<Action> Inline = action => action();

    private sealed record Card(
        PresetsViewModel Presets, FormSession Form, SeededBackend Backend, SetupViewModel Flow);

    /// <summary>
    /// A flow whose first form has landed and whose preset card has read the store once.
    /// Both fixtures answer from memory and the dispatcher runs inline, so what a test reads afterwards is
    /// what the render pass wrote.
    /// </summary>
    private static async Task<Card> CardAsync(SeededBackend? seeded = null)
    {
        var backend = seeded ?? new SeededBackend("linux");
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        var flow = new SetupViewModel(backend, form, session, Inline);

        await form.Settled;
        await flow.Review.Presets.Settled;

        return new Card(flow.Review.Presets, form, backend, flow);
    }

    /// <summary>The draft's way of publishing, which is what a save sends.</summary>
    private static PublishSettings Publish(Card card) => card.Form.Draft!.Publish;

    private static PresetRow Row(Card card, string name)
        => Assert.Single(card.Presets.Rows, row => row.Name == name);

    private static BuiltinPresetRow Builtin(Card card, string key)
        => Assert.Single(card.Presets.Builtin, row => row.Key == key);

    /// <summary>One field write, arriving as it does from a control the reader moved.</summary>
    private static void Write(Card card, string key, string value)
        => card.Form.Write(key, new FieldValue { Text = value });

    /// <summary>
    /// The rows are the store's, in the order it holds them, and the card reads it when it opens rather than
    /// waiting to be told: nothing on the contract announces a preset, so a card that only listened would
    /// show an empty list to a reader who has saved twenty.
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

    /// <summary>
    /// A store that answered and holds nothing says so.
    /// It is a state and not a failure, which is why it is a different sentence from the one an unreadable
    /// store carries.
    /// </summary>
    [Fact]
    public async Task AnEmptyStoreSaysNothingIsSavedYet()
    {
        var card = await CardAsync();

        Assert.Empty(card.Presets.Rows);
        Assert.True(card.Presets.IsEmpty);
        Assert.False(card.Presets.HasNotice);
    }

    /// <summary>
    /// The name is what a preset is, which is the whole reason the switch this card replaced could not work.
    /// What crosses is the draft's way of publishing, and the row that appears comes from reading the store
    /// again rather than from what was sent.
    /// </summary>
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
    /// A name already in the store replaces what is under it, and the button says so before the press.
    /// Saving over a preset is how one is edited - the name is the identity the store keys on - so the word
    /// on the button is the only warning there is.
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

    /// <summary>
    /// The box is empty once what was in it has been saved.
    /// Text left behind would offer to replace the preset that was just written, which is not what a reader
    /// who has finished naming something is about to do.
    /// </summary>
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
    /// A name with nothing in it saves nothing, and neither does one that is only spaces: the backend refuses
    /// an empty name, and a card that sent one would be asking for a refusal it could have answered itself.
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

    /// <summary>Deleting takes the row off, because the store is read again after it.</summary>
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
    /// Applying replaces the way of publishing whole and leaves the rest of the draft where it is.
    /// That boundary is what a preset means: the relay belongs to a deployment and the watch settings to this
    /// machine, so a preset that moved either would break exactly where it was meant to help
    /// (<c>docs/presets.md</c>).
    /// </summary>
    [Fact]
    public async Task ApplyingReplacesThePublishGroupAndLeavesTheOthersAlone()
    {
        var card = await CardAsync();

        // Saved from the draft with one field moved, so what comes back through the repair is the preset
        // itself rather than a walked-to-legal version of it.
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
    /// The row marked as the one in force is derived from the settings, so an edit that moves off the preset
    /// unmarks it.
    /// A snapshot carries no claim about a region of the settings space, so being selected can only mean
    /// being equal - and there is no stored selection left to disagree with the draft
    /// (<c>docs/presets.md</c>, "Saved presets").
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

    /// <summary>
    /// Applying puts nothing on the air and stores nothing.
    /// A way of publishing is staged until a commit carries it, so trying a preset out is free - which is
    /// what makes a list of them worth clicking through.
    /// </summary>
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
    /// A refused call is the backend's own sentence, shown as it stands, and it leaves the list alone: the
    /// rows on screen are still the last ones the store answered with, and the reader may well have fixed
    /// what the sentence named.
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

        // The name is still in the box, because the save it was typed for did not happen.
        Assert.Equal("lan", card.Presets.Name);
    }

    /// <summary>
    /// A preset another window deleted first is refused by name, and the row stays until the store is read
    /// again.
    /// Reading it from the failure would replace the sentence the reader has not read yet, so the re-read is
    /// theirs to press.
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
    /// A store that could not be read carries the backend's notice rather than the sentence for a store
    /// holding nothing.
    /// The two facts are different - nothing was saved, and nothing readable remained - and the notice is the
    /// one that says where the old file went.
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

    /// <summary>
    /// Two passes over an unchanged store produce rows that compare equal, which is what lets the review
    /// render on every keystroke and leave the list alone.
    /// </summary>
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
    // The other kind of preset entirely.
    // A saved one is a snapshot the user named; these are promises about the picture, resolved against this
    // machine by the backend, and every word about them is written here (docs/presets.md).

    /// <summary>
    /// The rows are the form's, in the order it offered them, and each carries the words this shell keeps for
    /// the identifier: which presets exist is not something a shell may know.
    /// </summary>
    [Fact]
    public async Task TheCardListsTheBuiltInPresetsTheFormCarried()
    {
        var card = await CardAsync();

        Assert.Equal(["lossless", "gaming", "readability"], card.Presets.Builtin.Select(row => row.Key));
        Assert.Equal("Gaming", Builtin(card, "gaming").Name);
        Assert.NotEqual("", Builtin(card, "gaming").Promise);
    }

    /// <summary>
    /// Applying writes what the backend resolved for this machine, and only the way of publishing: the relay
    /// stays where it is, and nothing is committed, so trying a preset out is free.
    /// </summary>
    [Fact]
    public async Task ApplyingABuiltInPresetWritesWhatTheBackendResolvedForThisMachine()
    {
        var card = await CardAsync();

        Write(card, "relay.host", "relay.example");
        await card.Form.Settled;

        var resolved = Builtin(card, "gaming");
        Assert.True(resolved.IsReachable);

        // The relay is an applied group, so the write above was persisted on its own.
        // What this counts is what the apply below adds to that, which is nothing.
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
    /// Applying a preset marks it, because the mark is derived from the settings on every resolve rather than
    /// remembered from the press.
    /// At most one is ever marked: the promises are written so that no settings deliver two of them.
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
    /// A field the promise says nothing about may move without leaving the preset, and one it does cover
    /// takes the mark off.
    /// That is the whole difference from a saved preset, which is selected only while the draft equals it
    /// field for field.
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
    /// A preset nothing on this machine reaches keeps its row, greyed, with the backend's reason under it -
    /// the treatment every other ruled-out choice gets.
    /// Pressing it does nothing: a repaired near miss would be a way of publishing the reader did not ask
    /// for, under the name of one they did.
    /// </summary>
    [Fact]
    public async Task APresetNothingHereReachesKeepsItsRowAndSaysWhy()
    {
        var card = await CardAsync();

        // A codec whose encoder takes no planar RGB, which is what the lossless promise rests on.
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
