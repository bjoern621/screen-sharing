using System.Globalization;
using ScreenShare.Api.V1;

namespace ScreenShare.App.Copy;

/// <summary>
/// Turns one statement from the backend into a sentence.
///
/// The backend never sends prose. It sends a code naming which fact it is stating and the
/// identifiers the fact is about - "this pair shares no device memory", capture
/// <c>portal</c>, codec <c>libx264</c>, engine <c>gstreamer</c> - and this is where that
/// becomes something to read (api/proto/screenshare/v1/text.proto).
///
/// Every sentence here follows the same shape, and it is the shape that makes a greyed
/// control useful rather than merely honest:
///
/// <b>Name the limit, name which side has it, name the way out.</b> "AV1 needs a recent
/// GPU" is a limit with no side and no exit. "The GStreamer encoders have no preset
/// ladder - switch to a capture method that runs ffmpeg to use it" says what is missing,
/// whose it is, and what to do. Where the backend hands over an alternative, the sentence
/// uses it; where it hands over none, the sentence stops rather than trailing off.
///
/// <b>Say it in the reader's terms.</b> The identifiers cross the wire because they are
/// what both sides agree on; they reach the screen through <see cref="Words"/>, so a
/// reader is told about an NVIDIA GPU rather than about <c>nvenc</c>. The identifier
/// itself comes back only where the reader will meet it again in a log or a command line.
///
/// An unknown code renders as the code. A shell older than its backend will meet one, and
/// a blank where a reason belongs reads as a control greyed for no reason at all - which
/// is worse than a line the reader can search for.
/// </summary>
public static class Statements
{
    /// <summary>
    /// The sentence for one statement, and the empty string for no statement at all.
    /// Absence is the normal case: most controls carry no reason and most entries no note.
    /// </summary>
    public static string Of(Text? text)
    {
        if (text is null || text.Code == TextCode.Unspecified)
        {
            return "";
        }

        var a = new Args(text);
        return text.Code switch
        {
            // --- Capture backends and monitors ------------------------------------

            TextCode.CaptureWrongOs =>
                $"{Words.Capture(a.Capture)} needs {Words.OperatingSystem(a.Os)}.",

            TextCode.CaptureWrongSession =>
                $"{Words.Capture(a.Capture)} needs an {Words.DisplayServer(a.Display)} session. "
                + "On Wayland it would see only the older windows, not the desktop - use the screen picker instead.",

            // A note and not a refusal, so it is a fragment: it prints on the entry's own row,
            // beside the name, where a sentence would crowd the name out of the width a
            // dropdown has. What the privilege is for is the entry's paragraph
            // (Descriptions.Capture).
            TextCode.CaptureNeedsGrant => "needs elevated privileges",

            TextCode.CaptureTakesNoMonitor => a.Capture switch
            {
                "kmsgrab" => "This capture takes everything the GPU is scanning out, not one screen.",
                "gdigrab" => "This capture takes the whole desktop as one picture - every monitor side by side.",
                "portal" => "The desktop's own picker chooses what is shared, so there is nothing to pick here.",
                _ => "This capture method chooses what it grabs itself.",
            },

            // Why the wizard offers a list of screens and no pictures of them. Two sessions
            // reach a screen only through something that chooses for them, and each is named
            // for what it does rather than for what it lacks: naming the picker and naming the
            // system is what tells a reader this is how their machine works and not a fault.
            TextCode.NoMonitorPreview => a.Display == "wayland"
                ? "Wayland reaches a screen only through the desktop's own picker, which asks "
                  + "every time, so there is no picture of one screen to show here. The picker "
                  + "shows what is being shared when the stream starts."
                : "This system hands the app whichever screen it likes rather than the one asked "
                  + "for, so there is no picture of a single screen to show here.",

            TextCode.MonitorNotEnumerated =>
                "Not currently connected. It is still selected, so you can see what the stream would capture - "
                + "pick a screen that is plugged in.",

            TextCode.ScaledFromSource =>
                $"from {Plain(a.Width)} × {Plain(a.Height)}",

            // --- Publish engines and the encoder probe ----------------------------

            TextCode.EngineToolingMissing =>
                $"{Words.Engine(a.Engine)} is not installed, so nothing that runs on it can encode here. "
                + "Install it, or pick a capture method that runs the other one.",

            TextCode.EngineHasNoPublishSink =>
                $"{Words.Capture(a.Capture)} runs on {Words.Engine(a.Engine)}, which cannot send over "
                + $"{Words.Transport(a.Transport)}. Change the capture method to reach it.",

            TextCode.EngineNotProbed =>
                $"Nothing on {Words.Engine(a.Engine)} has been tested on this machine, because "
                + $"{Lower(Of(a.Cause))} Until then nothing is greyed out for missing hardware, "
                + "so a choice this machine cannot run will fail when you start rather than here.",

            TextCode.ProbeNoDevice =>
                $"No {Words.Family(a.Family)} encoder was found on this machine. Either the hardware is not here, "
                + "or its driver does not offer encoding.",

            TextCode.ProbeNoBuild =>
                $"This install has no {a.Codec} encoder. It is a missing piece of software rather than missing "
                + "hardware, so another build or another package would bring it back.",

            TextCode.ProbeFailed =>
                $"{Words.Engine(a.Engine)} could not run {a.Codec} on this machine.",

            TextCode.CodecNotImplemented =>
                "Not built yet - it is listed so you can see it is coming.",

            // --- Carriage ---------------------------------------------------------

            TextCode.TransportCarriesNoCodec => Ways(
                $"{Words.Transport(a.Transport)} cannot carry this format on {Words.Engine(a.Engine)}",
                a.Transports.Count > 0
                    ? $"send it over {Words.List(a.Transports.Select(Words.Transport))}"
                    : "",
                a.OtherEngine.Length > 0
                    ? $"keep {Words.Transport(a.Transport)} and pick a capture method that runs {Words.Engine(a.OtherEngine)}"
                    : ""),

            TextCode.LegCarriesNoAudioCodec => a.AudioCodecs.Count > 0
                ? $"{Words.Transport(a.Transport)} carries no {Words.AudioCodec(a.AudioCodec)} track on "
                  + $"{Words.Engine(a.Engine)} - it carries {Words.List(a.AudioCodecs.Select(Words.AudioCodec))}."
                : $"{Words.Transport(a.Transport)} carries no audio at all on {Words.Engine(a.Engine)}. "
                  + "Send over another protocol, or turn audio off.",

            TextCode.RenderChainElementMissing =>
                $"This machine has no {a.Element}, which {Words.RenderChain(a.Value)} needs. "
                + "Another route converts the frames instead.",

            TextCode.EngineHasNoAudioEncoder =>
                $"{Words.Engine(a.Engine)} has no {Words.AudioCodec(a.AudioCodec)} encoder. "
                + "Change the capture method to reach the other engine.",

            TextCode.NoViewerReceivesOver =>
                $"No player here opens {Words.Transport(a.Transport)}. "
                + $"You can watch over {Words.List(a.Transports.Select(Words.Transport))}.",

            TextCode.RelayServesNoFormatOver => a.Transports.Count > 0
                ? $"The relay does not re-serve {Words.Format(a.Format)} over {Words.Transport(a.Transport)}, so a "
                  + $"player would connect and receive nothing. Watch over {Words.List(a.Transports.Select(Words.Transport))}."
                : $"The relay does not re-serve {Words.Format(a.Format)} over {Words.Transport(a.Transport)}, so a "
                  + "player would connect and receive nothing.",

            // --- Pixel format, colour and decoding --------------------------------

            TextCode.CodecCodesNoRgb =>
                "This encoder cannot take the desktop's pixels directly. Only H.265 and VP9 have a mode for it, "
                + "and only on some encoders - pick 4:4:4 for the next best thing.",

            TextCode.CodecCannotEncodeChroma =>
                $"This encoder cannot code {Words.Chroma(a.Chroma)}. Most hardware encoders do 4:2:0 and nothing "
                + "else; the CPU encoders reach the rest.",

            TextCode.RgbIsFullRange =>
                "RGB uses every code value by definition, so there is nothing to choose here.",

            TextCode.DecodesInHardware =>
                $"Viewers decode this on the GPU on {Words.List(a.DecodeFamilies.Select(Words.DecodeFamily), "and")}.",

            TextCode.DecodesOnCpu =>
                $"No GPU decodes {Words.Format(a.Format)} at {Words.Chroma(a.Chroma)}, so every viewer spends CPU on it"
                + (a.Decoder.Length > 0 ? $", through {a.Decoder}" : "")
                + ". That is a trade you are allowed to make - it plays everywhere, it just costs them.",

            TextCode.DecodesInHardwarePartly =>
                $"{Words.Chroma(a.Chroma)} {Words.Format(a.Format)} reaches a GPU on "
                + $"{Words.List(a.DecodeFamilies.Select(Words.DecodeFamily), "and")} and nowhere else. "
                + "Everyone else decodes it on the CPU, which still works and costs them cores.",

            // --- Frame memory and the GPU path ------------------------------------

            TextCode.PairHasNoDeviceMemory =>
                $"{Words.Capture(a.Capture)} and this encoder share no memory on {Words.Engine(a.Engine)}, "
                + "so the frames go through main memory whatever this is set to.",

            TextCode.PairConvertsOnDevice =>
                $"{Words.Capture(a.Capture)} and this encoder already convert on the GPU using the colour you chose, "
                + $"so there is nothing to trade. \"{Words.Memory(a.Memory)}\" keeps both.",

            TextCode.PairTradesColour => Sentences(
                $"{Words.Capture(a.Capture)} and this encoder can share memory on {Words.Engine(a.Engine)}, but "
                + "nothing between them can convert the colour",
                Of(a.Cost),
                Of(a.Reach)),

            TextCode.ExactColourReach =>
                $"{Words.Capture(a.Capture)} on {Words.Engine(a.Engine)} shares the same screen and does convert, "
                + "so it keeps the GPU path and the colour you chose.",

            TextCode.DevicePathHasNoScaler =>
                "On this path the frames never leave the GPU and nothing on it can resize them. "
                + $"Send at the source size, or set frames to \"{Words.Memory(a.Memory)}\".",

            TextCode.DrmMapUnusedOnDevice =>
                "The frames stay on the GPU, so nothing is copied back and no route is chosen.",

            // --- What carries the frames on each GPU path -------------------------

            TextCode.ImportGstPortalVaapi =>
                "The picker's frames are handed to the encoder directly, and the GPU converts them on the way.",

            TextCode.ImportGstD3D11Nvenc =>
                "The captured texture is converted on the GPU and handed straight to the NVIDIA encoder.",

            TextCode.ImportFfmpegKmsgrabVaapi =>
                "The scanout buffer is handed to the encoder as a GPU surface and converted there.",

            TextCode.ImportFfmpegDdagrabQsv =>
                "The captured texture is handed to Quick Sync as a GPU frame and converted there.",

            TextCode.ImportFfmpegDdagrabNvenc =>
                "The NVIDIA encoder reads the captured texture on its own device, with nothing converting in between.",

            TextCode.CostEncoderSignalsItsOwnColour =>
                $"The encoder converts the picture itself and sends {Words.Chroma(a.Chroma)} at "
                + $"{Words.ColorRange(a.ColorRange)} whatever you pick here.",

            // --- Rate control ------------------------------------------------------

            TextCode.CqOnlyInConstantQuality =>
                "There is a quality target only when the encoder is holding quality. Switch to constant quality to set one.",

            TextCode.BitrateNotInMode =>
                $"{Words.Mode(a.Mode)} aims at no bandwidth figure - it spends whatever the picture costs.",

            // --- The pointer -------------------------------------------------------

            TextCode.KmsgrabHasNoCursorPlane =>
                $"{Words.Capture(a.Capture)} reads the screen as it is scanned out, and the pointer is composed "
                + "over that at the last moment by the display hardware. There is nothing on that path to draw it in.",

            TextCode.CaptureHasNoCursorMetadata =>
                $"{Words.Capture(a.Capture)} hands over a picture and nothing else. Only the desktop portal reports "
                + "where the pointer is, separately from the frames.",

            TextCode.CursorMetadataNotCarried =>
                "Nothing carries the pointer's position to a viewer yet, and no viewer draws one, so a stream sent "
                + "this way would arrive with no pointer at all.",

            TextCode.CqAboveCodecScale =>
                $"{a.Codec} counts quality to {a.CqMax} on {Words.Engine(a.Engine)}, so the slider stops there. "
                + "Each encoder has its own scale, and the same number is a different quality on each.",

            TextCode.BitrateAboveCodecLimit =>
                $"{a.Codec} refuses a target above {a.BitrateLimitMbps} Mbit/s on {Words.Engine(a.Engine)}. "
                + "It is the encoder's own limit, and an encode asking for more does not start.",

            TextCode.MaxrateOnlyInConstrainedVbr =>
                "Only capped variable bitrate has a ceiling to raise. The other modes either hold the target or ignore it.",

            TextCode.VbvOnlyInBoundedModes =>
                "The buffer only means something while the encoder is holding a bandwidth figure.",

            TextCode.BframesOffInMode =>
                $"{Words.Mode(a.Mode)} switches these off: they would save nothing here and still add delay.",

            TextCode.BframesOnlyOnFamilies =>
                $"Only the {Words.List(a.Families.Select(Words.Family), "and")} encoders take these.",

            TextCode.PresetOnlyOnFamilies =>
                $"Only the {Words.List(a.Families.Select(Words.Family), "and")} encoders have this ladder.",

            TextCode.PresetPinnedByMode =>
                $"{Words.Mode(a.Mode)} pins this to {a.EncPreset} for its low-delay tuning.",

            TextCode.AudioCodecNeedsSource =>
                "No audio is being sent, so there is nothing to compress.",

            TextCode.AudioSourceUnserved => a.Os switch
            {
                "windows" => "Windows offers no way to record what it is playing that either encoder can open. "
                    + "A virtual audio cable is the usual workaround.",
                "darwin" => "macOS offers no system-audio input device, so neither encoder can record what it plays. "
                    + "A loopback driver is the usual workaround.",
                _ => $"{Words.OperatingSystem(a.Os)} offers nothing here that either encoder can record from.",
            },

            TextCode.AudioSourceServer => a.Os switch
            {
                "linux" => "through PulseAudio or PipeWire",
                _ => "",
            },

            TextCode.AudioTrackCodedAt =>
                $"{Decimal(a.RateHz / 1000.0)} kHz · {Number(a.BitrateKbps)} kbit/s",

            // --- Where an engine departs from the mode --------------------------------

            TextCode.GstNoPresetLadder =>
                "The GStreamer encoders have no effort ladder. Pick a capture method that runs ffmpeg to use it.",

            TextCode.Rav1ESizesNoRateBuffer =>
                "rav1e has no rate buffer, in any mode.",

            TextCode.GstRav1EncNoKeyframeInterval =>
                "The GStreamer rav1e element has no keyframe-interval setting, so its own default stands.",

            TextCode.GstNvencNoRateBuffer =>
                "The GStreamer NVIDIA elements have no rate-buffer setting.",

            TextCode.GstQsvNoRateBuffer =>
                "The GStreamer Quick Sync elements have no rate-buffer setting. It works on ffmpeg.",

            TextCode.NvencCqBitrateCapsBursts =>
                "In constant quality NVIDIA uses this as a ceiling on bursts. The quality target still drives the look.",

            TextCode.GstVpxCqBitrateIsCap =>
                "The GStreamer VP8 and VP9 elements have no unbounded quality mode, so this is the cap their quality "
                + "control stays under.",

            TextCode.FixedFunctionAbrDerivesCeiling =>
                "Hardware encoders always code against a ceiling, so this target is sent with twice itself as one. "
                + "The average is what the target holds.",

            TextCode.AmfCodesNoBframes =>
                "AMD's encoders are driven with look-ahead frames switched off, so a live stream pays none of their delay.",

            TextCode.VaapiCeilingBound =>
                $"On this encoder the ceiling cannot exceed {Number(a.MaxrateMbps)} Mbit/s against a "
                + $"{Decimal(a.BitrateMbps)} Mbit/s target - it sets the target as a percentage of the ceiling, "
                + "and half is as low as it goes.",

            // --- Codec capability gaps -----------------------------------------------

            TextCode.GapNvencAv1NoLosslessTune =>
                "NVIDIA's AV1 encoder has no lossless mode, unlike its H.264 and H.265 ones.",

            TextCode.GapGstVp9EncNoLossless =>
                "The GStreamer VP9 element has no lossless setting. It works on ffmpeg.",

            TextCode.GapVp8HasNoLossless =>
                "VP8 has no lossless mode at all - that arrived with VP9.",

            TextCode.GapGstAv1EncEightBitOnly =>
                "The GStreamer libaom element takes 8-bit input only. 10-bit works on ffmpeg.",

            TextCode.GapLibaomNoLosslessSwitch =>
                "Neither build exposes libaom's lossless switch.",

            TextCode.GapGstAv1EncNoColourDescription =>
                "The GStreamer libaom element writes no colour information, so every viewer would expand the picture "
                + "as limited range whatever you pick. It works on ffmpeg.",

            TextCode.GapSvtav1NoLossless =>
                "SVT-AV1 has no lossless mode.",

            TextCode.GapSvtav1NoConstrainedVbr =>
                "SVT-AV1 refuses a ceiling outside quality mode, so no build can cap its bursts. "
                + "Average bitrate is the same encode under a name that fits it.",

            TextCode.GapGstSvtav1EncNoCbr =>
                "The GStreamer SVT-AV1 element stalls in the mode constant bitrate needs. It works on ffmpeg.",

            TextCode.GapRav1ENoLossless =>
                "rav1e has no lossless mode.",

            TextCode.GapRav1ENoConstrainedVbr =>
                "rav1e takes one bitrate target and nothing above it, so no build can cap its bursts. "
                + "Average bitrate is the same encode under a name that fits it.",

            TextCode.GapAmfAv1LimitedRangeOnly =>
                "AMD's AV1 encoder marks everything as limited range whatever it is given, so a full-range stream "
                + "would arrive stretched at every viewer.",

            TextCode.GapVulkanAv1LimitedRangeOnly =>
                "The Vulkan AV1 encoder marks everything as limited range whatever it is given, so a full-range "
                + "stream would arrive stretched at every viewer.",

            TextCode.GapVaapiNoLossless =>
                "Intel and AMD's fixed-function encoders quantize every frame - no profile they implement is lossless.",

            TextCode.GapGstVaNoColourDescription =>
                "The GStreamer VAAPI elements write no colour information, so every viewer would expand the picture "
                + "as limited range whatever you pick. It works on ffmpeg.",

            TextCode.GapQsvNoLossless =>
                "Quick Sync quantizes every frame - Intel's runtime exposes no lossless path.",

            TextCode.GapGstAmfcodecWindowsOnly =>
                "The GStreamer AMD plugin is built for Windows only. AMD's encoders are reachable through ffmpeg.",

            TextCode.GapAmfNoLossless =>
                "AMD's fixed-function encoders quantize every frame - no profile they implement is lossless.",

            TextCode.GapGstVulkanNoCaptureMemory =>
                "The GStreamer Vulkan encoder reads frames no capture method on that engine produces. "
                + "Vulkan encoding is reachable through ffmpeg.",

            TextCode.GapVulkanNoLossless =>
                "Vulkan's lossless setting is a hint rather than a mode, and its encoders quantize under it anyway.",

            TextCode.GapVp8HasNoColourRangeField =>
                "VP8 has nowhere in its stream to record the colour range, so every viewer would expand the picture "
                + "as limited range. The other formats carry it and reach both.",

            TextCode.GapGstSoftwareNoRateCeiling =>
                "No GStreamer CPU encoder takes a ceiling above the target, so capped variable bitrate needs ffmpeg. "
                + "Average bitrate is the same encode under a name that fits it.",

            TextCode.GapGstElementsNoPlanarRgb =>
                "The GStreamer H.265, VP9 and AV1 elements take no RGB input. Sending the desktop's own pixels "
                + "needs a capture method that runs ffmpeg.",

            // --- Diagnostics ---------------------------------------------------------

            TextCode.PublishRefused =>
                "These settings cannot be published as they stand.",

            TextCode.NoUplinkStated =>
                "No upload speed is set, so nothing checks the stream against your connection. Measure it or type it "
                + "in, and a configuration this line cannot carry will say so here instead of at your viewers.",

            TextCode.UplinkBelowPrediction =>
                $"This is predicted to need about {Decimal(a.BitrateMbps)} Mbit/s, and your upload speed is "
                + $"{Number(a.UplinkMbps)} Mbit/s. The encoder does not slow down to fit: the stream queues, packets "
                + "are dropped, and your viewers see it stall rather than soften.",

            TextCode.BurstAboveUplink =>
                $"These settings run between about {Decimal(a.LowMbps)} and {Decimal(a.HighMbps)} Mbit/s depending on "
                + $"what is on screen, and the top of that is above your {Number(a.UplinkMbps)} Mbit/s upload speed. "
                + $"{Burst(a.Mode)} A still desktop will go out fine and a moving one will not.",

            TextCode.FpsAboveRefresh =>
                $"You are asking for {Number(a.Fps)} frames a second from a screen that produces "
                + $"{Number(a.RefreshHz)}. The extra frames are copies of the last one: they cost bandwidth and add "
                + "no smoothness.",

            TextCode.MonitorNotPriced =>
                "The selected screen is not connected, so there is no picture size to predict from.",

            TextCode.NoPictureToPrice =>
                "These settings do not add up to a picture that can be predicted from.",

            TextCode.CompressionRatio => a.BitrateMbps > 0
                ? $"Your screen produces {Decimal(a.RawMbps)} Mbit/s uncompressed, and this is predicted to send "
                  + $"{Decimal(a.BitrateMbps)} Mbit/s of it - about {Number((long)Math.Round(a.RawMbps / a.BitrateMbps))}:1."
                : $"Your screen produces {Decimal(a.RawMbps)} Mbit/s uncompressed.",

            // --- Presets ---------------------------------------------------------------

            // The sentence names the preset even though it prints under the row that already
            // does, so that it still says what it is about wherever it is shown. The publish
            // leg is the way out the backend hands over: it is the one dimension the search
            // does not move, so it is what is left to change.
            TextCode.PresetUnreachable =>
                $"Nothing this machine can run delivers {Words.Preset(a.Preset)} over "
                + $"{Words.Transport(a.Transport)}. Nothing was changed - a near miss under this "
                + "name would be settings you did not ask for.",

            // --- Notices ---------------------------------------------------------------

            TextCode.SettingsStoreUnreadable => a.Path.Length > 0
                ? $"Your saved settings could not be read, so these are the defaults. The old file was kept as {a.Path}."
                : "Your saved settings could not be read, so these are the defaults.",

            TextCode.PresetStoreUnreadable => a.Path.Length > 0
                ? $"Your saved presets could not be read, so none are listed. The old file was kept as {a.Path}."
                : "Your saved presets could not be read, so none are listed.",

            // A backend newer than this build. The code is shown so it can be searched for
            // and reported, which is more than a blank line gives anyone.
            _ => text.Code.ToString(),
        };
    }

    /// <summary>Whether a statement would render as anything at all.</summary>
    public static bool Any(Text? text) => Of(text).Length > 0;

    /// <summary>
    /// Why a mode's bandwidth spreads, for the sentence that says the top of the spread is
    /// too high. It follows the mode alone, so it sits beside the sentence quoting it
    /// rather than becoming three sentences.
    /// </summary>
    private static string Burst(string mode) => mode switch
    {
        "crf" => "Constant quality sets no bandwidth bound at all, so what goes out is whatever the picture costs.",
        "lossless" => "A lossless encode spends whatever the picture costs and approaches the raw rate on motion.",
        "vbr" => "Variable bitrate averages toward the target and bursts to the ceiling above it.",
        _ => "",
    };

    /// <summary>
    /// A statement and the ways out it carries: "…, but X - or Y." A statement with no way
    /// out ends after the fact rather than promising one.
    /// </summary>
    private static string Ways(string fact, params string[] ways)
    {
        var offered = ways.Where(way => way.Length > 0).ToList();
        return offered.Count == 0 ? fact + "." : $"{fact} - {Words.List(offered)}.";
    }

    /// <summary>
    /// Several statements read one after another, dropping the ones that had nothing to
    /// say. Each already ends in its own punctuation, so they are joined with a space.
    /// </summary>
    private static string Sentences(string opening, params string[] rest)
    {
        var parts = new List<string> { opening.EndsWith('.') ? opening : opening + "." };
        parts.AddRange(rest.Where(part => part.Length > 0));
        return string.Join(" ", parts);
    }

    /// <summary>A whole figure, grouped the way the reader's own locale groups one.</summary>
    private static string Number(long value) => value.ToString("N0", CultureInfo.CurrentCulture);

    /// <summary>
    /// A whole figure with no grouping at all, for the ones that are identifiers rather
    /// than quantities: a pixel dimension is written 2560 everywhere in the world, and a
    /// thousands separator in one would read as a different number.
    /// </summary>
    private static string Plain(long value) => value.ToString(CultureInfo.InvariantCulture);

    /// <summary>
    /// A rate, to one decimal place under ten and to none above it. A stream is never
    /// precise to a hundredth of a megabit and printing one implies it is.
    /// </summary>
    private static string Decimal(double value) =>
        value < 10 ? value.ToString("0.#", CultureInfo.CurrentCulture) : value.ToString("0", CultureInfo.CurrentCulture);

    /// <summary>
    /// A sentence lowercased at its first letter, for quoting one inside another. Only the
    /// first character moves, so an identifier or an acronym further in is left alone.
    /// </summary>
    private static string Lower(string sentence) =>
        sentence.Length == 0 || char.IsLower(sentence[0]) ? sentence : char.ToLowerInvariant(sentence[0]) + sentence[1..];

    /// <summary>
    /// The arguments of one statement, read by name.
    ///
    /// By name and never by position, because a statement carries the arguments its facts
    /// needed and leaves out the ones they did not: the engine a way out points at is
    /// present on some rows and absent on others, and a reader keyed on position would
    /// silently shift.
    /// </summary>
    private readonly struct Args(Text text)
    {
        private readonly Text _text = text;

        public string Capture => Id(TextArgName.Capture);

        public string Engine => Id(TextArgName.Engine);

        public string OtherEngine => Id(TextArgName.OtherEngine);

        public string Transport => Id(TextArgName.Transport);

        /// <summary>The built-in preset a statement is about, which is not the NVENC ladder step
        /// <see cref="EncPreset"/> carries.</summary>
        public string Preset => Id(TextArgName.Preset);

        public string Codec => Id(TextArgName.Codec);

        public string Format => Id(TextArgName.Format);

        public string Family => Id(TextArgName.Family);

        public string Chroma => Id(TextArgName.Chroma);

        public string ColorRange => Id(TextArgName.ColorRange);

        public string Mode => Id(TextArgName.Mode);

        public string Memory => Id(TextArgName.Memory);

        public string AudioCodec => Id(TextArgName.AudioCodec);

        public string EncPreset => Id(TextArgName.EncPreset);

        public string Decoder => Id(TextArgName.Decoder);

        public string Os => Id(TextArgName.Os);

        public string Display => Id(TextArgName.Display);

        public string Path => Id(TextArgName.Path);

        public string Value => Id(TextArgName.Value);

        public string Element => Id(TextArgName.Element);

        public IReadOnlyList<string> Families => Ids(TextArgName.Families);

        public IReadOnlyList<string> Transports => Ids(TextArgName.Transports);

        public IReadOnlyList<string> AudioCodecs => Ids(TextArgName.AudioCodecs);

        public IReadOnlyList<string> DecodeFamilies => Ids(TextArgName.DecodeFamilies);

        public long Width => Num(TextArgName.Width);

        public long Height => Num(TextArgName.Height);

        public long Fps => Num(TextArgName.Fps);

        public long RefreshHz => Num(TextArgName.RefreshHz);

        public long MaxrateMbps => Num(TextArgName.MaxrateMbps);

        /// <summary>The top of the selected codec's quantizer scale on the engine behind the capture.</summary>
        public long CqMax => Num(TextArgName.CqMax);

        /// <summary>The highest bitrate target the selected codec's encoder accepts.</summary>
        public long BitrateLimitMbps => Num(TextArgName.BitrateLimitMbps);

        public long UplinkMbps => Num(TextArgName.UplinkMbps);

        public long RateHz => Num(TextArgName.RateHz);

        public long BitrateKbps => Num(TextArgName.BitrateKbps);

        /// <summary>
        /// The bitrate, which arrives as a whole figure where it is a target the user set
        /// and as a fraction where it is a prediction. Both are read here, so a sentence
        /// quoting one does not have to know which of the two it got.
        /// </summary>
        public double BitrateMbps => Dec(TextArgName.BitrateMbps);

        public double RawMbps => Dec(TextArgName.RawMbps);

        public double LowMbps => Dec(TextArgName.LowMbps);

        public double HighMbps => Dec(TextArgName.HighMbps);

        public Text? Cause => Nested(TextArgName.Cause);

        public Text? Cost => Nested(TextArgName.Cost);

        public Text? Reach => Nested(TextArgName.Reach);

        private string Id(TextArgName name) => Find(name)?.Id ?? "";

        private IReadOnlyList<string> Ids(TextArgName name) => Find(name)?.Ids?.Ids ?? (IReadOnlyList<string>)[];

        private long Num(TextArgName name)
        {
            var arg = Find(name);
            return arg?.ValueCase switch
            {
                TextArg.ValueOneofCase.Number => arg.Number,
                TextArg.ValueOneofCase.Decimal => (long)Math.Round(arg.Decimal),
                _ => 0,
            };
        }

        private double Dec(TextArgName name)
        {
            var arg = Find(name);
            return arg?.ValueCase switch
            {
                TextArg.ValueOneofCase.Decimal => arg.Decimal,
                TextArg.ValueOneofCase.Number => arg.Number,
                _ => 0,
            };
        }

        private Text? Nested(TextArgName name) => Find(name)?.Text;

        private TextArg? Find(TextArgName name)
        {
            foreach (var arg in _text.Args)
            {
                if (arg.Name == name)
                {
                    return arg;
                }
            }

            return null;
        }
    }
}
