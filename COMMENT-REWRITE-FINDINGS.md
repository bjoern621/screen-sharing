## batch 01 (catalog.proto, control.proto)
- Fixed false claim in control.proto service comment: said Subscribe was "the one server-streaming method"; there are three (Subscribe, SubscribeAudioLevels, SubscribePointer).
- Fixed stale cross-reference in catalog.proto TransportList: pointed at "WatchTransportsByFormat", now names Catalog.watch_transports_by_format.
- OUT OF SCOPE / doc drift: docs/ipc-api.md method-kind table lists only Subscribe + SubscribeAudioLevels under Stream; omits SubscribePointer and MeasureEncodeRate.
- POSSIBLE DEFECT (not fixed): Monitor.offset_x/offset_y are plain int32 (no presence) but comment says absent on platforms with no virtual-desktop origin. On the wire that is 0, not absent. Either field wants `optional` or comment wants "zero".
- api/gen/go is stale wrt proto comments -> central regen at the end.
## batch 03 (frame.proto, session.proto, settings.proto)
- Fixed wrong field reference: FrameRelease comment pointed at FrameEvent.dropped; real field is FrameReady.dropped.
- Fixed wrong reserved-range claims: PublishState reserved comment said "1 through 6"; only 2-6 reserved, 1 carries Live. Same class of error in PublishSettings reserved list (31 and 34 are live fields inside the stated "30 to 36").
- REVIEW LATER: dangling MoqCert comment at end of session.proto documents a message that does not exist. Agent kept it, rewritten as statement of finished state. Consider deleting.
## batch 06 (SeededBackend.cs and screen-picker/setup-resolve tests)
- CatalogAsync had two stacked <summary> blocks; the first actually documented Defaults(). Moved.
- Comment/code contradiction: FrameRateOptions summary claimed rates above panel refresh are greyed; code sets no Reason on any rate, and the inline comment said the opposite of the summary. Comment now matches code.
- Comment/code contradiction: KeyframeOptions doc claimed the label is the concept and a Note carries the frame count; no Note is set on any keyframe option and the value IS the count. Comment now matches code.
- DEAD DATA (not fixed): PlatformOf's tuple carries a second member WrongOs that nothing reads; refusal crosses as TextCode.CaptureWrongOs.

## batch 07 (viewer/tile/setup tests)
- Agent briefly renamed ViewerBackend.ResolveFormAsync while deleting a comment, caught and reverted. Confirms need for central comment-stripped diff audit.
- Kept several documented shipped defects as comments (step-table keys naming groups no backend answers, watch group in sending wizard, pop-out close-is-a-state, stats-overlay hover chrome).

## batch 08 (Backend/*.cs, viewer roster/vocabulary/watchsettings tests)
- Removed a count that would rot: "the group's six knobs" in WatchSettingsTests.
- Kept documented idempotency departure on ApplyToStream (names a transition on purpose; repeat is a second restart).
## batch 02 (events.proto, form.proto)
- Fixed stale tally in events.proto header: claimed "the three rates carry presence"; four fields do. Tally dropped.
- CONTRACT CONTRADICTION (needs decision): text.proto names exactly three raw strings (Summary.command, ExitInfo.message, RelayStatus.error), but Summary.command_error and ReceiveStreamStats.codec_description each claimed to be "one of the three". Agent dropped the tally in both places. text.proto's list is short by at least two -> check batch 04's result and fix the list.
## batch 04 (text.proto, App.axaml(.cs), Program.cs, 5 test files)
- DELETED stale/false block in App.axaml.cs: claimed tile grid gone and "nothing renders a frame yet to arrange". TileGrid.cs and ViewerView.axaml.cs exist.
- text.proto: dropped ENC_PRESET rename archeology.

## main-thread fix
- text.proto header claimed "Three strings stay raw"; Summary.command_error is a fourth. Rewrote as a rule, no count.

## batch 10 (IBackend.cs, PresetStore.cs)
- IBackend summary contradicted docs/ipc-api.md: said what each thing is called "arrives through here already decided". ipc-api.md says every word on screen is the shell's. Rewritten.
- "Changed" carried dated count "One thing raises it today" -> restated as invariant.
- Recovered facts the C# interface was missing vs control.proto: idempotency departure on OpenInBrowser/OpenLog/OpenLogsFolder, volume units+range (bare double had no range documented), StartReceive same-toneMap idempotency, monitor index meaning, audio-level cadence 15/s coalesced.
## batch 09 (Backend/FieldValues, FormSession, FrameChannel, FrameDescriptors)
- ADDED missing unit: FrameDescriptors.PollInterval is microseconds (Socket.Poll); bare 100_000 showed nothing.
- ADDED missing contract fact: zero dimension on render size leaves the pipeline alone (frame.proto FrameRenderSize).
- Rotted count fixed: FrameChannel class summary said "which of the two", named only relay decode + publish preview; OpenMonitorAsync is a third arm.

## batch 11 (Session.cs, SettingsDraft.cs, Assert.cs, CheckItem/*)
- Session.cs: two stacked <summary> blocks above PointerAsync; the meter loop doc was orphaned there and MeterAsync had none. Moved.
- Session.cs Metered doc said raised after Levels moved "and never by anything else"; Meter() also writes Pointer. Corrected.
- Rotted count: Session.cs Start said "Two loops rather than one" while three run (RunAsync, MeterAsync, PointerAsync).
- Rotted count: Session.cs LoadAsync said "Reads the three states" while it reads six.
- Rotted count: CheckItem.GlyphOf said "a fourth state fails here first" with five enum members.
## batch 12 (Controls/*, Copy/*)
- THIRD stacked <summary>: Copy/Cards.cs had two on ScreenOpening; screen-picker paragraph was orphaned there and ScreenPickerCost had no doc.
- STALE RATIONALE (needs your decision): Copy/Fields.cs `Unit` justified the separate unit word with "the design sets figures in mono and everything else in sans". docs/design-language.md says the second family is gone and alignment comes from FigureFeatures (tabular figures). Agent did not restate the false reason; new line states empty-string behaviour. The wording RULE itself may now be unmotivated.
- Deleted change-log prose: PreviewLocalCost "it used to say the opposite", NudgeCaveat old caption, ConfigReadOnly, Toggle 12px-vs-11px mockup archeology.
## batch 14 (HeaderStats, Broadcast/Model, Nudge)
- REAL BUG (not fixed, code change): HeaderStatsViewModel.Retry and IsRetrying are set on every pass but NO view binds them. HeaderStatsView.axaml binds only Elapsed, IsSharing, Figures. A stream in retry backoff shows a pill identical to a healthy one. Same defect class NudgeViewModel's own comment documents for IsEnabled/Reason.
- STYLE VIOLATION in shipped string (not fixed, string literal): HeaderStatsViewModel.Apply builds user-facing copy inline ($"reconnecting - attempt {..} of {..}") instead of from Copy/, and that literal contains a U+2014 em-dash. Global rule bans em-dash anywhere a human reads.
- Deleted archeology: BroadcastSnapshot "it used to be a list of field names", NudgeViewModel "for a while it drew neither".
## batch 15 (Plots, Preview)
- LIKELY DEFECT (not fixed): PreviewView.axaml "level meter" is five Rectangles with hardcoded heights (3.85/6.6/9.35/5.5/2.75) and NO binding. Old comment claimed it shows "the encoder is moving"; it shows nothing. Mockup-number-as-measurement class that avalonia/README.md says was purged elsewhere. Comment now says it is decoration.
- DEAD MARKUP (not fixed): PlotsView.axaml latency card Grid ColumnDefinitions="27*,73*" positions a congestion-band label, but BandStart/BandWidth are never bound and Band is empty. Band can never draw.
- ADDED missing unit: Sparkline.BandWidth had none while sibling BandStart did.
- EM-DASH in shipped UI copy: PlotsView.axaml legend Text="— rtt" / Text="— loss" (U+2014), and they are on-screen copy living in the view instead of Copy/.
## batch 13 (Copy/Vocabulary, Words, Design/*.axaml, ConfigCard)
- FACTUAL CORRECTION: Words.Preset doc said the collision with Effort is because it is "a step of the NVENC ladder". Efforts and Presets share no identifier; real collision is the word "preset", which x264 uses for its effort ladder.
- ADDED fact table cannot show: DecodeFamilies keys are the decoders' own ("va"), not Families' ("vaapi").
- STALE refs to DELETED surfaces (dropped, not rewritten): Metrics.axaml StripTileHeight cited "the height the two existing viewers already use"; Icons.axaml cited the web frontend and GTK grid taking the same Tabler set. docs/viewer-architecture.md records both surfaces as deleted.
- Design/Palette.axaml 26 comments -> 7; Design/Text.axaml 22 -> 8. Nearly all were role-to-English translations of the resource key.

## structural markers kept byte-identical by consensus
- `// --- Inputs ---` / `// --- Outputs ---` in view models (24 C# files use the pair) and `<!-- Surfaces --> `-style section markers in Design/*.axaml. Region markers, not prose.
## batch 16 (PreviewViewModel, BroadcastViewModel, SessionLog, BroadcastView)
- Clean. No defects. Dropped <b> emphasis tags that wrapped rhetorical topic sentences.

## batch 17 (Fields/*, AdvancedDrawer, ViewerTable)
- FALSE, backwards from contract: OptionViewModel claimed "Every string here was written by the backend." Label/Detail come from Vocabulary, Note/Reason from Statements: all shell-side. Predates the rule move in docs/ipc-api.md.
- FALSE: AdvancedDrawerView.axaml called the note column "the form's own help text". Help comes from Copy.Fields.Of(Key).
- SELF-CONTRADICTION: ViewerTableViewModel.Apply said the roster is "pushed unchanged every five seconds"; that feature's own view comment says no poll period is on the contract. Interval dropped.
- STALE: AdvancedDrawerView.axaml.cs documented "resetting the table", a control the drawer no longer has.
- EM-DASH + copy-outside-Copy/: AdvancedDrawerView.axaml Text="note - why you would touch it" (U+2014); also Text="Advanced" and two Notice sentences in ViewerTableViewModel.Apply live outside ScreenShare.App/Copy.
## batch 19 (Presets, QualityStep, ReviewStep, ScreenPicker)
- REAL BUG (not fixed, code change): QualityStepView.axaml quantizer band ratios "29*,12*,22*,37*" and end labels ("0 ...", "51 - unusable") are hardcoded to the 0..51 H.26x scale, but the slider's Minimum/Maximum bind to Field.Range, which the backend states per codec/engine (0..63 libvpx and software AV1; 0..127 or 0..255 for raw-quantizer-index encoders per docs/glossary.md, docs/video-stack.md). On those codecs the coloured zones AND both end labels describe a scale the track is not on.
- ScreenChoice label format now shown by example: "Screen 1 - 2560 x 1440 - 144 Hz - main", rate absent where the output reports none.
## batch 18 (CostRail, Setup/Model, Presets)
- SUSPECTED RENDERING BUG (not fixed): CostRailView.axaml sets FillBrush="{StaticResource ForegroundBrush}" inline on MeterBar. Inline = LocalValue, which outranks the `view|MeterBar.over` style setter in Avalonia binding priority, so the over-uplink attention hue likely never paints. Fix: move default FillBrush into a base style.
- CONTRACT-PLACEMENT DEFECT (not fixed): MeterBar.Render puts Assert.That(thickness > 0, ...) AFTER the drawing instead of before it. CLAUDE.md requires a precondition at the top of the function before any work.
- Rotted count: CostMetricRow doc claimed the rail carries three priced dimensions ("bitrate, latency and GPU load"); CostRailViewModel.Rows builds two (uncompressed capture, uplink headroom).
- EM-DASH in shipped copy: PreflightChecks.Clear "Nothing to fix - these settings publish as they stand." (U+2014)
## batch 20 (ScreenPickerViewModel, StepStrip, SetupView)
- FIVE more rotted counts, all replaced by the invariant: StepStripView.axaml "walk the seven in order" and "a strip of seven chips"; SetupView.axaml "Five of the seven are one component"; StepChipViewModel "Four states rather than a done flag"; ScreenPickerViewModel "three different facts" and "All four are read through".
  Note: the step tallies were false-able by construction. SetupSteps.For derives one step per Form.groups entry plus the terminal one, so no number belongs in a comment there at all.
- MALFORMED XML DOC: ScreenPickerViewModel constructor had `</param> <param name="dispatch">` run together on one line, so two parameter docs read as one wrapped block. Separated.
## batch 21 (SetupViewModel, Shell/Model/*)
- STACKED <summary> #4: SetupViewModel FieldOf carried two; first documented GroupOf which had none. Deleted (GroupOf's name+signature say it all).
- EIGHT rotted counts: Destination "The three places"; StepContent "Three, and ... every one renders a group of the resolved form" (ALSO FALSE, Review renders no group); class summary "three of the seven rows ... four groups unreachable"; _session "two of the four things the commit turns on"; gate comment "four states ... three of them decide"; Summaries "four invented ones"; ToplevelPresence "The three facts" + "the three properties"; WindowChrome "read in three places ... answered three times".
- UNBOUND PROPERTY (not fixed): SetupViewModel.Headline is set on every pass, no .axaml binds it (only two tests read it). Its comment was ALSO wrong: claimed the backend composes it; it comes from shell-side Copy/Vocabulary.Headline. Attribution corrected.

## batch 22 (Shell NavStrip/StatusBar/TitleBar/ShellWindow)
- STACKED blocks #5: ShellWindow.axaml had two <!-- --> on TitleBarView; first documented the drag region (a fact TitleBarView.axaml already states). Merged.
- FIVE rotted counts: NavStripView "The three destinations" (strip is built from Destinations.All); NavStripViewModel "The pair the whole strip turns on" over three asserts; ShellViewModel "// --- The three destinations ---" banner; ShellWindow.axaml "Four rows: two chrome bands..." and "Two of the three state a minimum".
- UNBOUND OUTPUTS (not fixed): ShellViewModel.Current and ShellViewModel.IsBroadcastAvailable are public, raise change notification on every write, and nothing binds or reads either (no axaml ref, no test ref).
- SUSPECTED LOST BINDING (not fixed): TitleBarView.axaml 12px mark documented as "the app mark, and the live indicator", but the Border has no binding and is unconditionally AttentionBrush, the one red docs/design-language.md reserves for sharing and failure. Either markup lost a visibility binding or the comment was aspirational.
- DEAD ANCHOR: StatusBarViewModel cited scratchpad/spec/6a-nav-chrome.md, not in the tree. Dropped.
## batch 25 (TileViewModel, TileGrid, ViewerView)
- STACKED <summary> #6 and #7, both in TileViewModel.cs: SendAudioAsync's doc sat orphaned above SendToneMapAsync; NoticeFor's ("Why this tile is dark") sat orphaned above ColourNoteFor. In both cases the described member had none.
- WRONG CROSS-REFERENCE: CanToneMap said a machine that cannot convert "keeps the row greyed with ColourNote saying what is absent". ColourNote is the badge over the picture; the greyed row's sentence is ToneMapNote.
- Rotted counts: menu block said "the five flags above and beside this block" but rows read back SIX (IsFocused, IsPoppedOut, IsFullscreen, Muted, ToneMapped, ShowStats); also "forty rows" stats panel, "six tiles" in ShowStats, "three states" in NoticeFor, "two curves are HDR" in IsHdr.
- UNBOUND (but not dead): TileViewModel.IsHdr set by Apply with change notification, no markup binds it. Still read inside Apply for CanToneMap, asserted, exercised by ToneMapTests. Unbound, not dead.

## batch 27 (WatchSettings, Mvvm/*)
- MY BRIEF WAS WRONG, agent corrected it: Reconcile has NO key-based identity rule. It is SequenceEqual over records then clear-then-fill, so identity is a row's value AT ITS POSITION and a rebuild is all-or-nothing. A moved row is handled exactly as a changed row.
- ADDED thread fact Observable.cs lacked: notification raised on whichever thread wrote; a binding tolerates the UI loop alone, so an off-loop answer is marshalled before the write.
- Rotted counts: PendingCommand "the same three properties", "written in exactly two places"; axaml "Three rows"; IsUnkept "Two things raise it".
- GAP left as-is: WatchSettingsViewModel constructor documents dispatch and close but not form and session.
## batch 23 (Viewer/Model, Tile/Model, PopOut)
- COMMENT DESCRIBED OPPOSITE BEHAVIOUR: TileLayout.Best fallback said tiles go "in the most rows that still fit the width"; loop breaks on the FIRST row count that fits = the FEWEST. Code correct, comment inverted.
- Rotted count: TilePipeline "One shape with two producers"; three factories exist (ReceiveStream, PublishState.Live.preview, PreviewedMonitor).
- WRITTEN-NEVER-READ (not fixed): PopOutWindow.Stream (old doc claimed it exists "so a reconciling pass can tell its windows apart"; ViewerView.axaml.cs keys a Dictionary<string,PopOutWindow> instead); TilePipeline.Chain/RenderMemory/Decoder/Hardware (all three factories fill them, no view reads them; TileStats reads the same figures off ReceiveStreamStats); TileSource.IsPreview (only BroadcastPreviewTests; production uses IsRelay).
- ADDED missing geometry facts: lengths are Avalonia layout units; rectangles in the box's space, origin top-left.

## batch 26 (Viewer/ViewModel/*)
- MAJOR STALE CLASS DOC: ViewerViewModel summary opened "this is a roster and not a tile grid" and "frames are a second channel that does not exist yet". Grid, TileViewModel and the frame channel are ALL live. Also carried archeology about the deleted GTK4 grid button.
- FALSE: BrowserLegViewModel class summary had it backwards ("the value and the label are the backend's"); the word is shell-side, the leg is the catalog's.
- FALSE: WatchLegViewModel.Label said "written in Go"; labels come from Copy.Words.Transport on the shell side.
- FALSE: WatchLegViewModel said a per-stream refusal is shown by the row; it lands in ViewerViewModel.Refusal, drawn at the foot of the rail.
- FALSE: Hint's doc said "this screen has no tiles" while the string it documents reads "Right-click a tile for focus, pop-out and volume...".
- FALSE: FiguresFor said "this shell receives nothing"; it does, via tiles.
- UNBOUND: ViewerViewModel.HasTiles written every Apply pass, bound by no axaml, referenced by no test/code. Doc claimed it "separates the grid from its empty state"; ViewerView.axaml has no such state. False claim deleted, code left.
- MISPLACED DOC: ToggleWatchSettings carried "names the state rather than the transition", which describes CloseWatchSettings; the code under it toggles.
## batch 24 (Tile/View: DmaBufSurface, SharedTextureSurface, StreamTile, TileCard, TileKeys)
- ADDED SAFETY FACT the file lacked: SharedTextureSurface keyed-mutex pair is CROSSED (acquire on the pool's consumer key, release on its producer key; both zero on a handle type synchronized otherwise). From FramePool.producer_key/consumer_key.
- ADDED: the SupportedImageHandleTypes refusal is deliberate, not a fallback to a system-memory copy.
- Rotted counts (six): TileCard.axaml "Three groups ... where this stream is drawn, how loud it is, and what is printed over it" (markup has FOUR separator-delimited groups; tone mapping was added and never covered); same block "a menu of seven equal rows" and "parsing seven verbs"; TileKeys.cs enumerated "the three arrangements, the mute, the stats overlay, and the pair that moves the volume"; StreamTile.Ladder "every distinct ask costs three allocations" (contract guarantees a pool of at least TWO slots); TileCard.axaml.cs "three things the markup cannot state"; DmaBufSurface.Shader "differ in three keywords" (rewrite also swaps gl_FragColor and the //DECLAREGLFRAG marker).
- STALE: colour-badge block positioned the badge "opposite the figures rather than above them, so the strip that grows with the pipeline..."; that figures strip is GONE (the same file's header block says so).
- UNBOUND #NEW (not fixed): TileViewModel.IsLive written on every Apply pass, nothing binds or reads it anywhere in app or tests. (Distinct from IsHdr, which batch 25 found and which IS read inside Apply.)
## batch 27b (cmd/backend, cmd/groupd, flake.nix, internal/app core)
- DEAD TYPE (not fixed): internal/app/events.go `watchExitEvent` has NO reference anywhere in the tree, and the "watch:exit" event it claimed to be the payload of does not exist. Viewer exits go out as wire.ViewerExitEvent from internal/app/watch.go.
- IDEMPOTENCY GAP (real, not fixed): App.Start is NOT wholly idempotent. `go a.startTestStreamsAtBoot()` carries no sync.Once, unlike startControl, startRelayPoll and startReceiveStatsPoll. Comment now names the three guarded ones instead of claiming all of Start.
- DELETED-FRONTEND REFERENCES: control.go header explained the adapter as a concession to "the Wails binding generator"; events.go narrated the removed second surface; control_test.go said "the frontend renders"; control.go spoke of "the window's lifecycle" and "this window" for a HEADLESS process. All rewritten.
  STILL PRESENT elsewhere (covered by batches 29/30): internal/app/watch.go:338, system.go:26, settings.go:46 and :77, watch_test.go:20.
- flake.nix measurement-posing-as-doc: "needs AMF 1.4.37 or newer, which is what the package set carries" - trailing clause falsified by any nixpkgs bump. Requirement kept, reading dropped.
- Rotted counts: app.go "Three mutexes guard the mutable state", "One case reaches it"; control.go "idempotent three times over", "the flat form's four booleans"; events.go "the three together separate a stream...".
## batch 29 (internal/app receive/settings/system/teststreams)
- DEAD CODE (not fixed): App.GetPresets, App.TransportFormats, App.GridTransports, App.CaptureTransports, App.CaptureEngines have NO caller anywhere outside settings.go. The control backend exposes none of them.
- MORE deleted-frontend justifications, all rewritten: GetPresets justified its empty-slice return by "a nil slice would cross the Wails boundary as JSON null"; TransportCarriage justified its flat shape by "the Wails generator reaches a struct through a return type and not through a map value"; TransportFormats/GridTransports/CaptureTransports/CaptureEngines named "the native-grid verdict", "the grid button", "the Live table", "the frontend"; system.go header had a paragraph on the Wails binding generator having no model for context.Context.
- ADDED missing departure doc: OpenLog / OpenLogsFolder are a documented idempotency departure (docs/development-principles.md "Effects across a process boundary") and NO comment said so.
- COMMENT NAMED A FUNCTION THAT DOES NOT EXIST: doc on TestTheTestStreamBackoffGrows began "TestTheBackoffGrows".
- POSSIBLE UNDOCUMENTED PARADIGM DEPARTURE (design question for owner): StartTestStreams replaces a running set, so it names a COUNT rather than converging one slot to a desired state. Rewritten doc now says so explicitly.
- Rotted counts: "the three figures" in TestUnnegotiatedFiguresStayAbsent (checks four fields), "three x264 encoders" and "as a fourth stream" in teststreams.go (both tied to testStreamsAtBoot = 3), "the three long-running calls" in system.go.
## batch 28 (internal/app groups/monitorpreview/pointer/preview/publish/retry)
- DEAD CODE (not fixed): App.startPublishHeld and App.Publishing have NO callers anywhere in the tree (the Publishing() calls in internal/control resolve to wire.PublishSnapshot.Publishing). Their doc comments existed to describe callers that no longer exist.
- COMMENT CONTRADICTED CODE: emitPublishState claimed state is "read once and announced twice" to "both surfaces". a.emit publishes ONE event on ONE broker.
- OVERCLAIMING COMMENT CORRECTED (subtle, about locks): stopPreviewLocked said clearing the field before receiver.Stop() keeps other callers from waiting behind the teardown. procMu is held by the caller ACROSS that Stop(), so every other procMu path DOES wait. Kept only the true half: an exit reported during teardown finds no preview to end.
- STALE FILE REFERENCE: publish.go cited app_publish_retry.go; the file is publish_retry.go.
- WRONG DOC IDENTIFIER: monitorPreviewLeg const doc began "previewLeg for a screen is...", naming the constant in preview.go.
- MORE deleted-surface references: three sites in publish.go (startPublishHeld, emitPublishState, Publishing) described "the native grid's publish button" and "the tray". The GTK4 nativegrid binary is gone.
## batch 32 (capabilities/rules, colour, control/effects+errors+frames+hello+levels+listen)
- CONTRACT DISCREPANCY (real, needs a decision): control.proto says StartMonitorPreview answers INVALID_ARGUMENT for a monitor no output is enumerated under, and FAILED_PRECONDITION for a session that cannot read screens apart. effects.go wraps EVERY backend error as FAILED_PRECONDITION, so the INVALID_ARGUMENT case CANNOT occur. Either proto or code is wrong; either way Backend needs a typed refusal.
  Same root cause as the already-documented StartWatch gap (a transport this build has no viewer for should be INVALID_ARGUMENT).
- ADDED missing unit: control/levels.go had no dBFS statement. "A figure is dBFS: at most zero, and silence negative infinity" previously existed only in wire/session.go and session.proto.
- ADDED several gRPC statuses that were only implied: SavePreset empty name, StartWatch/StopWatch half-pair, StartPublish different-pipeline.
- ADDED explicit idempotency + three documented departures in effects.go (ApplyToStream names a transition; OpenInBrowser/OpenLog/OpenLogsFolder open a second window).
- Status snapshot removed: rules.go "The table declares none today, because both codecs reach both engines"; kept the invariant it justified.
- Rotted counts: rules_test.go "libsvtav1 is the one row with a bitrate ceiling", "the form narrowed the bitrate even in the two modes that send none".
## batch 34 (cursor, display, encoderate, encoders, events/broker)  *** HIGHEST-VALUE FINDING ***
- REAL BUG, CONFIRMED BY ME: internal/display/wayland.go:74
    var wlrPositionRe = regexp.MustCompile(`Position:\s*(\d+),(\d+)`)
  No minus sign. wlr-randr reports a NEGATIVE position for an output left of or above the layout
  origin ("Position: -1920,0"). That output silently keeps offset 0,0, and crop-based capture then
  grabs the wrong rectangle. x11.go has the same pattern shape but X screen coords are non-negative,
  so only the Wayland path is exposed. Fix: `Position:\s*(-?\d+),(-?\d+)`.
- DUPLICATE CODE (not fixed): internal/screensrc/screensrc.go monitorAt is a byte-for-byte duplicate of display.At.
- PERF, documented not fixed: display.At re-runs List per call = one subprocess (xrandr/hyprctl/wlr-randr) per call on Linux, and callers hit it repeatedly.
- ADDED coordinate space that was missing: display_windows.go rcMonitor is in virtual-screen space, origin at the primary output's top-left, so offsets go negative left of or above it. (Directly relevant to the wayland bug above.)
- ADDED lock ownership in events/broker.go: mu guards the map, nextID, and each subscriber's sequence/dropped.
- ADDED probe cost in encoders.go: one child process per probed codec, all at once, cost is the slowest not the sum; nothing cached here, the process-lifetime cache is internal/app's.

## batch 31 (capabilities codecs/decoders/ladders)
- FALSE, deleted: av1_vulkan said "RTSP alone, as on every AV1 row". transport.Formats carries AV1 on RTSP, RTMP and HLS, and carriage is not a codec-table column at all.
- FALSE, deleted: libvpx-vp9 called "the one 4:4:4 codec a browser decodes in software (WebCodecs)". Nothing in these tables states a browser verdict and the WebKit viewer does not do 4:4:4 WebCodecs.
- STALE ANCHOR: librav1e pointed at "the frontend's engine rules"; that departure lives in internal/form/availability.go availabilityEngineRules.
- POSSIBLE TABLE INCONSISTENCY (not fixed): 4:2:2 note claimed "AV1's is the professional profile 2, which libaom codes". libaom-av1 row lists no yuv422p and dav1ddec carries none, so adding it would fail TestEveryPublishableStreamHasADecoder. Comment now matches the table. If libaom IS meant to reach 4:2:2, both tables need the format.
- Neither recorded capability defect (p010 verdicts, h264_nvenc p010) had a stale comment asserting it.
## batch 36 (ffmpeg encoders/gpu/hwsurface/kmsgrab/job)  *** DOC BUG, VERIFIED BY ME ***
- FALSE STATEMENT IN docs/domain-model.md:73 (the authoritative doc):
    "The QSV and AMF rows declare neither: both builders still spend a constant on those families'
     scales, because a ladder has to be read off the encoder rather than declared from memory."
  Verified against internal/capabilities/codecs.go: 4 QSV rows carry
  `Effort: Ladder{Steps: qsvTargetUsages, Defaults: qsvTargetUsageDefaults}` and 3 AMF rows carry
  `Effort: Ladder{Steps: amfPresets, Defaults: amfPresetDefaults}`. Both builders read l.Effort.
  The same stale sentence had been copied into the qsvArgs and amfArgs comments; those are fixed.
  docs/ is Markdown prose and out of the comment-rewrite scope, so line 73 is UNFIXED. Needs a decision.
- DEAD CONSTANTS (not fixed): amfLivePreset and amfQualityPreset in ffmpeg/encoders.go are referenced by
  nothing; the same two strings live in amfPresetDefaults in capabilities/ladders.go.
  (qsvLivePreset/qsvQualityPreset ARE used, inside qsvPresets.)
- TEST COMMENT OVERSTATED ITS TEST: TestTheGpuPathReadsNoDrmDownloadStrategy claimed it covered "a value
  the table does not carry", but it sets DrmMap = "vulkan", which DrmMaps DOES carry.

## batch 35 (ffmpeg args)
- DEAD SYMBOLS in comments: ENGINE_RULES exists nowhere (rules moved to form.availabilityEngineRules);
  VaapiFilters exists nowhere (the chroma mapping is HwSurfaceFilters, checked by TestHwSurfaceFilters).
- WRONG OWNER named: captureBackends pointed at publish.Captures (names only); the platform column is
  publish.captureNeeds. avfNoAudioDevice said the refusal lives in audioInputArgs; it is publish.AudioAvailable.

## batch 37 (ffmpeg probe/proc/progress/qsv/tail/vaapi/vulkan, form/audio)
- ADDED the ffmpeg progress protocol outright: one key=value per line, blocks terminated by
  progress=continue and the run's last by progress=end, one block per stats period (0.5 s unless
  -stats_period says otherwise), SO A SAMPLE'S INTERVAL IS THAT PERIOD, NOT A FRAME TIME.
- ADDED QSV vendor requirement + its ambiguity: session opens only where a oneVPL runtime is installed
  (loaded by filename off distro library paths or ONEVPL_SEARCH_PATH); A MISSING RUNTIME LOOKS IDENTICAL
  TO NO INTEL GPU (implementation list with no hardware entry).
## batch 38 (form/availability.go - the most heavily commented file in the repo)
- 542 -> 445 comment lines (-18%), 109 -> 106 blocks. Test file 156 -> 134.
- STALE COMMENT CONTRADICTING CODE: availabilityEngineRule's doc claimed "the GStreamer nvcodec elements
  take no effort step". publish/gstencoders.go passes `preset=`+l.Effort to them, the rules table's OWN
  comment says they take the same p1-p7 ladder, and TestBothLaddersReachBothEngines asserts it.
  Its other two examples ("x264enc cannot raise a ceiling above its bitrate", "vpxenc has no unbounded
  constant-quality mode") have no rows either; per domain-model.md those are capability MODE gaps, so
  they were dropped rather than restated unverified.
- PERF CLAIM THAT DOES NOT HOLD (worth a look if resolve cost matters): the old `verdicts` field comment
  said "evaluated once per resolve". availabilityOf calls verdictsOf on EVERY fieldState/optionState call,
  so the whole rule set is re-evaluated per field question, not per resolve.
- Time-bound wording fixed: audioDeviceReason said "Nothing is refused today".
- Rotted count: the `availability` struct doc said "the three facts"; it derives four (verdicts, engine,
  codec row, pair row) plus the entry pair.

## batch 41 (form/options.go, presets.go)
- PHANTOM SYMBOL: presetTable's comment and the init doc both named `presetInit`, which does not exist
  (the function is a plain `init`).
- Counts -> invariants tied to the deciding table: "the four memory settings" -> every value
  gpupath.Memories declares; "all three pointer modes" -> every mode cursor.Modes declares.
- Migration narratives deleted: options.go header "face table per axis"; presets.go util/presets.ts +
  util/presetSearch.ts moving into Go.
## batch 44 (netspeed, platform, pointer, portal, publish availability/cursor/ffmpeg) *** BUG #2, VERIFIED BY ME ***
- DATA RACE, CONFIRMED: internal/portal/dbus.go:155-162
      var tokenSeq uint64
      func newToken() string { tokenSeq++; return fmt.Sprintf("screenshare%d", tokenSeq) }
  Unguarded increment on a package-level counter. Two concurrent portal.Open calls race; a duplicated
  token collides on the D-Bus Request object path. The comment's own claim ("unique within the process")
  is exactly what the race breaks. Fix: atomic.AddUint64, or a mutex.
- TEST NAME DOES NOT MATCH BODY (not fixed): platform/audio_test.go TestAServedSourceNamesWhatServesIt
  only asserts that an ABSENT source names nothing. Nothing in it checks that a served source names a
  server. The old comment stated the converse of the code; new comment matches the body. Test name is
  still misleading.
- Rotted count: platform/audio.go audioSourceNeed claimed four columns ("a source, the platforms serving
  it, what serves it on each, and why the others do not"); the struct has TWO fields, the other two being
  derived statements.
- ADDED fact the file lacked: portal.go node is damage-driven, its PipeWire clock stops while the screen
  is still, so the consumer paces and re-stamps frames itself (points at publish/gstcapture.go imagefreeze).

## batch 45 (publish gstbundle/gstcontrol/gstcapture + audio/colorimetry tests)
- ADDED lock ownership that was nowhere stated: gstcontrol.go "mu guards conn, which an apply dials on
  first use and a failed write drops."
- Rotted counts: rateCaps "On the two backends whose Feature is empty..."; d3d11Capture.HoldsOneDevice
  "the two ffmpeg rows" -> "the ddagrab rows".
- EM-DASHES in a doc comment (fixed): gstRoundTripEncode carried two U+2014.

## batch 40 (form/form.go, keys.go, groups.go)
- POSSIBLE CODE BUG (comment vs code, needs decision): form.go resolveGroups claimed "a hidden field
  contributes nothing ... the form has no reason to name" it. resolveGroups filters NOTHING; an
  availability-hidden field is rendered with Visible: false, and live_test.go + availability_test.go both
  rely on that. Either the comment was aspirational or the code was meant to drop them.
- FALSE ON BOTH HALVES: keys.go said "watch" holds fields from two of the three settings messages and
  "relay" holds the relay group plus the stream name. GroupWatch is six viewer.* keys, GroupRelay is ten
  relay.* keys, and publish.name sits in GroupStream.
- Rotted count contradicting the product: form.go said "the app has three shells"; docs/ipc-api.md says one.

## batch 43 (group, groupclient, groupsvc, gstbundle, gstrun)
- COMMENT CONTRADICTED CODE: gstrun/pointer_test.go said the pipeline "ends itself, which is what stops
  the reporting". Description is `videotestsrc ! video/x-raw,framerate=30/1 ! fakesink` with NO
  num-buffers; what ends the run is the 500 ms context.WithTimeout.
- Go convention violations fixed: several groupclient doc comments did not start with the identifier
  they document.
## batch 39 (form diagnostics/estimate/facts/fields)
- DEAD CONSTANT + FALSE CLAIM (not fixed): form/fields.go fieldAnchorCq is referenced by nothing, and its
  old comment claimed it was "the range a codec declaring no scale is offered within". fieldCqBounds uses
  capabilities.WidestCqScale() instead.
- DEAD FUNCTION (not fixed): form/estimate.go estimateFigure has no caller anywhere in the tree.
- PHANTOM SYMBOL: estimate.go CBR/ABR/VBR arm pointed at (estimatePeak); the function is estimateSpread.
- Provenance deleted: "ported from the Wails frontend's util/estimate.ts".

## batch 42 (form repair/statements/summary, gpupath)
- FALSE IN TWO FILES: gpupath.go Paths claimed "Every row here converts on the device ... no pair the table
  currently carries asks the user to choose between hardware and colour", and gpupath_test.go said "Every
  shipping row converts on the device today ... these four answers are unreached code". The ffmpeg
  ddagrab/NVENC ColourEncoder row contradicts both.
- Rotted count wrong twice over: "The two ffmpeg rows map the captured frames onto a device derived from
  the frames themselves" - there are THREE ffmpeg rows, and the ddagrab/NVENC one derives nothing
  (gpuConverts has an empty NVENC entry).
- POSSIBLE CONTRACT SMELL (not fixed): form/statements.go argBitrateTarget(int) and argBitrateMbps(float64)
  BOTH fill TEXT_ARG_NAME_BITRATE_MBPS with different value types. One substitution name, two shapes
  (Number vs Decimal) depending on which constructor ran.
- POSSIBLE DEAD EXPORT (not fixed): gpupath.Mismatch referenced only by its own test; only Undetermined has
  a production caller (publish/gstcapture.go).

## batch 47 (publish gstlive/gstpipeline/gstprobe/gstreamer)  *** POSSIBLE RACE #3 ***
- POSSIBLE DATA RACE (not fixed, check with `go test -race`): gstEngine.Start writes `handle` and `stopped`
  on the Start goroutine while the stdout reader goroutine that supervise starts reads `handle` and writes
  `stopped`, with no synchronization. The old comment reasoned only about ordering (caps follow PLAYING so
  the nil window is not hit); the unsynchronized access itself would trip -race if caps ever arrive during
  startup.
- COMMENT CONTRADICTED CODE AND A SIBLING FILE: GstExe's doc claimed the gst-launch binary "is supervised as
  a child process exactly like ffmpeg" and that the encode probe runs through it. A publish spawns the binary
  itself (FindGstRunner + GstSubcommand); GstExe is reached only through FindGstExe, used by test streams,
  internal/encoderate and tests.
- STALE: gstAudioMixElement documented as "what a property write addresses". No live write addresses the
  mixer; gstLiveGainWrite addresses the per-source gain<N> volume elements.
- STALE: gstAudioBranch's doc described pulsesrc/monitor-device work that now lives in gstAudioSource.
- Rotted count that was ALSO wrong: gstlive.go table doc said "the two engines spell one figure four ways";
  every row in that map is a GStreamer element.
## batch 46 (publish gstencoders/gstgpu/gsthdr)
- COMMENT CONTRADICTED CODE: x265Encoder's doc claimed `zerolatency` on both the lossless and cbr branches.
  Neither sets it (lossless is option-string=lossless=1 only; cbr is bitrate + vbv-maxrate/vbv-bufsize).
  Zerolatency travels as a tune step.
- PHANTOM SYMBOL: the av1_qsv/vp9_qsv row comment referenced qsvShortBitrate; it is qsvShortBitrateKbps.
- Rotted count: gstReadChild's doc said "the two readers"; it serves three (meter, onCaps, onPointer).

## batch 48 (publish gststats/hdrrules/live/preview/publish/supervise)
- STALE PACKAGE DOC (worst place for one): publish.go said the GStreamer engine "drives the portal capture
  backend". captureBackends registers FOUR GStreamer backends (portal, ximagesrc, avfvideosrc,
  d3d11screencapturesrc).
- BROKEN MID-SENTENCE + wrong count: preview.go previewCarriage listed three conditions then said "All five
  are true here", with the reference split as "the same three transport.\n// Carriage is written against".
- Deleted archeology that had become FALSE: live.go bitrate row said it was "the socket's first customer
  rather than the audio gain the design named"; the audio gain is now a live row directly beneath it.

## batch 49 (publish teststream, receive audio/caps/chains/descriptors/elements)
- COMMENT CONTRADICTED CODE (needs a decision): Chains() claimed the offers come "in the table's order: the
  default first, then the device chains, then the unconverted one". Table order is cpu, gl, d3d11, d3d12,
  raw and DefaultChain is gl (or d3d11 on Windows), so THE DEFAULT IS NEVER FIRST. Rewritten to the true
  fact (table order, unconverted last). Either the ordering or the sentence was the intended contract.
- LATENT COUPLING (not a comment issue, not fixed): receive/audio.go `levelMessage = "level"` is documented
  as the bus message name but is ALSO used as a factory name to locate the element inside audioChain
  (slices.Index(audioChain, levelMessage)). One constant carrying two meanings; renaming either breaks the
  other silently.
## batch 52 (rules, screensrc, settings audio/migrate/notice/portal/presets)
- Rotted count that was wrong against BOTH code and docs: rules.go said "The three are the treatments
  docs/field-availability.md describes". FOUR verdicts exist (Refuse, Note, Hide, Live), and
  docs/field-availability.md itself names TWO treatments plus a note it explicitly calls "not a third".
- VERIFIED BY ME: axis.go's kept claim that a form test holds the axis list and the settings-field keys to
  each other is TRUE. internal/form/keys_test.go has TestAxesSpellTheFieldKeysTheyName and
  TestEveryFieldAxisIsAFieldTheFormDeclares.
- VERIFIED BY ME: internal/screensrc/screensrc.go ALREADY IMPORTS internal/display (line 33), so its
  byte-for-byte duplicate monitorAt is straightforwardly removable in favour of display.At.
- ADDED explicit Umgebungsfehler wording on settings.LoadPresets, which the code path already implemented
  via setAside.
## batch 53 (settings resolution/settings/store, text, token) *** SECURITY FINDING, VERIFIED BY ME ***
- WORLD-READABLE SECRETS: internal/settings/store.go:21 `const storeFileMode = 0o644`, used at line 145
  `os.WriteFile(path, data, storeFileMode)`. settings.json holds:
    - Relay.GroupKey     - its own comment: "the secret whose possession is membership of a group"
    - Relay.SrtPassphrase - its own comment: "whether the packets are readable at all"
  Meanwhile cmd/groupd/main.go:144 writes its ECDSA signing key with 0o600.
  On a shared machine any local user can read another user's group key and SRT passphrase.
  The agent declined to invent a justification in the comment, which is right. Needs a decision: 0o600.
- COMMENT CONTRADICTED CODE: token.go GroupPermissions claimed the grant expression is "anchored at both
  ends of the prefix"; the code prepends `~^` only and the assert checks just HasPrefix(p.Path, "~^").
  token_test.go repeated the claim in its header while asserting only the start anchor. Both corrected to
  "anchored at the start"; the guarantee still holds because the prefix carries its trailing `/`.
- CONFLICTING COMMENTS RESOLVED: resolution_test.go header claimed every value reaching ParseSize was
  written by this side (so malformed = caller bug), while resolution.go says values return through a
  user-editable file and malformed is an Umgebungsfehler. Kept the Umgebungsfehler framing.
- Copied constant removed: Sign now names groupsvc.TokenWindow instead of restating "five minutes".
- ADDED idempotency fact on store.Save: whole file written from the argument, so twice equals once.
## batch 50 (receive export/memory/pbutils/receive/receiver/share*)
- COMMENT ASSERTED THE OPPOSITE OF ITS CODE: share_windows.go said "A DXGI texture describes its own extent,
  and its rows run downward. Neither figure is this side's to state, so neither is stated." TopLeftOrigin:
  true IS stated, in that same literal.
- INCOMPLETE ENUMERATION: share.go header listed only share_windows.go and share_other.go, omitting
  share_linux.go which exists. Replaced with the invariant (one file per platform, each the only file
  naming its graphics API).
- ADDED fd-ownership fact share_linux.go did not state: the socket only LENDS (a send duplicates the
  descriptor into the consumer); closing this side's stays screenshare_share_close's job.
- ASYMMETRY (not fixed): dmabufSharer.open asserts slots > 0 as a precondition; d3d11Sharer.open has no
  equivalent.
- DISCIPLINE NOTE: agent found that inserting a comment line between TopLeftOrigin and ProducerKey in a
  composite literal RE-GROUPS gofmt's key alignment = a code change. Folded the note above instead.

## batch 51 (receive stats/tonemap/shader, relay)
- ADDED the SRT relay floor where it belongs: statsources.go negotiated-latency-ms now records that
  MediaMTX floors every hop at 120 ms with no config key, so a setting under 120 changes nothing AND READS
  BACK AS 120.
- ADDED lock ownership: statsread.go r.mu guards the fields onElement writes; handles copied under it,
  pipeline queried outside it.
- ADDED units statsources.go had none of: cumulative counts vs Mbit/s rates vs ms.
- Left byte-identical, correctly: the GLSL inside tonemapshader.go's raw string (ST 2084 / BT.2408 / matrix
  comments are shader source), and readers.go's verified protocol-to-figure table (data, not prose).

## batch 55 (transport/webrtc, watch, wire catalog/events/frame/pointer/doc)
- COMMENT CONTRADICTED CODE: wire/frame.go header claimed a handle kind with no contract value "asserts".
  frameHandleTypeOf returns FRAME_HANDLE_TYPE_UNSPECIFIED, and its own comment explains why it does not
  panic. Header claim dropped.
- DEAD SYMBOL: watch/players.go cited "the native grid" as the other consumer of RtspWatchProtocol. That
  grid is gone. Rewritten to name the receiving pipeline, where the setting actually lands
  (transport/rtsp.go GstSource, protocols=).
- ADDED whole-state-never-a-delta to wire/events.go header; it was only implicit.

## batch 56 (wire session/settings, nix)
- DEAD FIELD IN A COMMENT: session_test.go named PublishStats.missing, a field the proto has since reserved
  and removed.
- COMMENT CONTRADICTED CODE: audioSources said an empty list "crosses as an empty list rather than as an
  absent field"; the function returns nil, which encodes as ABSENT.
- Rotted counts: ReceiveStreamStats "the three rates are pointers" (eight fields carry presence, four are
  rates); PublishStats "those three counts".
