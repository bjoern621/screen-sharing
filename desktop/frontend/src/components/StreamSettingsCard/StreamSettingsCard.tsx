import {
    IconCheck, IconX, IconPlugConnected, IconDeviceDesktop,
    IconVideo, IconGauge, IconNetwork,
} from "@tabler/icons-react";
import { ReactNode, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { FieldSet, FieldLegend } from "@/components/ui/field";
import {
    Deps,
    Monitor,
    Option,
    Stream,
    ViewVerdict,
} from "../../types/stream";
import {
    AUDIO_SOURCES, CAPTURES, CHROMAS, DRM_MAPS, ENC_PRESETS, MODES,
    RANGES, TRANSPORT_META, bitrateTip, clampFps, codecOptions, cqTip,
    engineTip, engineValue, familyOptions, fpsDisabled, fpsOptions,
    monitorOptions, withNote, cropNote,
} from "../../util/options";
import {
    Capability, Engine, bitrateLimit, cqMax, familyOf,
} from "../../util/domain";
import { estimateBitrate, formatEstimate } from "../../util/estimate";
import Tip from "../Tip/Tip";
import ErrorLog from "../ErrorLog/ErrorLog";
import ReadonlyField from "../fields/ReadonlyField";
import SelectField from "../fields/SelectField";
import NumberField from "../fields/NumberField";
import NumberSelectField from "../fields/NumberSelectField";
import TextField from "../fields/TextField";
import UplinkField from "../fields/UplinkField";

/** Auto-fitting field grid shared by every settings section. */
const GRID =
    "grid gap-x-5 gap-y-3 [grid-template-columns:repeat(auto-fit,minmax(230px,1fr))]";

/** Viewability badges carry sentences, so they drop the badge's one-line height
 * clamp and align the icon to the first line instead of the block's centre. */
const VERDICT_BADGE =
    "h-auto items-start rounded-lg py-1 text-left whitespace-normal [&>svg]:mt-0.5";

/** One titled settings section: an icon+label legend over a field grid. */
function Section({
    icon,
    title,
    children,
}: {
    icon: ReactNode;
    title: string;
    children: ReactNode;
}) {
    return (
        <FieldSet>
            <FieldLegend className="flex items-center gap-2">
                {icon}
                {title}
            </FieldLegend>
            <div className={GRID}>{children}</div>
        </FieldSet>
    );
}

interface UplinkState {
    measuring: boolean;
    error: string;
    remeasure: () => void;
}

interface StreamSettingsCardProps {
    s: Stream;
    deps: Deps;
    caps: Capability[] | null;
    transports: string[];
    /** Publish engine of the selected capture backend, null until it resolves. */
    engine: Engine | null;
    monitors: Monitor[];
    webGrid: ViewVerdict;
    nativeGrid: ViewVerdict;
    cmd: string;
    publishing: boolean;
    pubError: string;
    pubLogPath: string;
    uplink: UplinkState;
    onUpdate: (patch: Partial<Stream>) => void;
    onTogglePublish: () => void;
    onSavePreset: (name: string) => void;
    onOpenLog: (path: string) => void;
    onOpenLogsFolder: () => void;
}

/** The full stream-settings form: dependency-aware fields, publish controls, the
 * web- and native-grid viewability verdicts and the live command preview. */
export default function StreamSettingsCard({
    s,
    deps,
    caps,
    transports,
    engine,
    monitors,
    webGrid,
    nativeGrid,
    cmd,
    publishing,
    pubError,
    pubLogPath,
    uplink,
    onUpdate,
    onTogglePublish,
    onSavePreset,
    onOpenLog,
    onOpenLogsFolder,
}: StreamSettingsCardProps) {
    const [presetName, setPresetName] = useState("");
    const saveDisabled = presetName.trim() === "";
    const savePreset = () => {
        if (saveDisabled) {
            return;
        }
        onSavePreset(presetName);
        setPresetName("");
    };

    const transportOptions: Option[] = transports.map(
        t => TRANSPORT_META[t] ?? { value: t, label: t, tip: t }
    );

    const selectedMonitor = monitors.find(m => m.index === s.monitor);
    // The engine decides whether the VBR ceiling the estimate would quote is one
    // the builder actually sends, so the estimate reads the same engine rules the
    // form greys its fields from.
    const estimate = estimateBitrate(
        s,
        selectedMonitor?.width ?? 0,
        selectedMonitor?.height ?? 0,
        caps,
        engine
    );

    // The codec picker is two dropdowns over one setting: the encoder family and
    // the video format within it. Picking a family jumps to that family's first
    // implemented codec; normalize then repairs chroma/transport if needed.
    const family = familyOf(s.codec, caps) ?? "";
    const selectFamily = (fam: string) => {
        const pick =
            caps?.find(c => c.family === fam && c.implemented) ??
            caps?.find(c => c.family === fam);
        if (pick) {
            onUpdate({ codec: pick.name });
        }
    };
    const resLabel =
        selectedMonitor && selectedMonitor.width && selectedMonitor.height
            ? `${selectedMonitor.width}×${selectedMonitor.height} @ ${s.fps} fps`
            : `@ ${s.fps} fps`;

    return (
        <Card>
            <CardHeader>
                <CardTitle className="text-base">Stream settings</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
                {publishing && (
                    <p className="text-xs text-muted-foreground">
                        Settings are locked while publishing. Stop the stream to
                        change them.
                    </p>
                )}
                {/* Changing settings mid-stream is unsupported, so the whole
                 * form is disabled while a stream is live. A disabled fieldset
                 * propagates the disabled state to every native input and select
                 * trigger inside it. */}
                <fieldset disabled={publishing} className="space-y-4">
                <Section
                    icon={<IconPlugConnected size={16} className="text-muted-foreground" />}
                    title="Connection"
                >
                    <TextField
                        label="Stream name"
                        labelTip="Path name on the relay. Friends see and open this name."
                        value={s.name}
                        onChange={v => onUpdate({ name: v })}
                    />
                    <TextField
                        label="Relay host"
                        labelTip="Host running the MediaMTX relay. All publishers push to it, all viewers pull from it."
                        value={s.relayHost}
                        onChange={v => onUpdate({ relayHost: v })}
                    />
                    {/* Each publish transport exposes only its own relay listener
                     * port: a backend implementation knob is hidden while its
                     * transport is not selected (docs/field-availability.md).
                     * A viewer's port follows its own watch leg, which the relay
                     * serves on all listeners regardless of this choice, so a
                     * protocol with no publish form at all takes the watch
                     * selection as its condition instead. */}
                    {s.transport === "srt" && (
                        <NumberField
                            label="Relay port (SRT, UDP)"
                            labelTip="UDP port of the relay's SRT listener (default 8890)."
                            value={s.relayPort}
                            onChange={v => onUpdate({ relayPort: v })}
                        />
                    )}
                    {s.transport === "rtsp" && (
                        <NumberField
                            label="Relay port (RTSP, TCP)"
                            labelTip="TCP port of the relay's RTSP listener (default 8554)."
                            value={s.rtspPort}
                            onChange={v => onUpdate({ rtspPort: v })}
                        />
                    )}
                    {s.transport === "webrtc" && (
                        <NumberField
                            label="Relay port (WebRTC, HTTP)"
                            labelTip="TCP port of the relay's WebRTC listener (default 8889), which serves both the WHIP publish endpoint and the WHEP playback one."
                            value={s.webrtcPort}
                            onChange={v => onUpdate({ webrtcPort: v })}
                        />
                    )}
                    {s.transport === "rtmp" && (
                        <NumberField
                            label="Relay port (RTMP, TCP)"
                            labelTip="TCP port of the relay's RTMP listener (default 1935)."
                            value={s.rtmpPort}
                            onChange={v => onUpdate({ rtmpPort: v })}
                        />
                    )}
                    {s.watchTransport === "hls" && (
                        <NumberField
                            label="Relay port (HLS, HTTP)"
                            labelTip="TCP port of the relay's HLS listener (default 8888). It follows the watch leg rather than the publish one: the relay serves HLS and ingests nothing over it, so nothing is published this way."
                            value={s.hlsPort}
                            onChange={v => onUpdate({ hlsPort: v })}
                        />
                    )}
                    <NumberField
                        label="Relay API port (HTTP)"
                        labelTip="TCP port of the relay's HTTP API (default 9997), used for the Live-now list."
                        value={s.apiPort}
                        onChange={v => onUpdate({ apiPort: v })}
                    />
                </Section>

                <Section
                    icon={<IconDeviceDesktop size={16} className="text-muted-foreground" />}
                    title="Source"
                >
                    <SelectField
                        label="Capture backend"
                        labelTip="How frames leave the desktop and reach the encoder. It also fixes the publish engine below, so it is the choice the rest of the form follows from."
                        value={s.capture}
                        options={CAPTURES}
                        optionDisabled={deps.optionDisabled.capture}
                        onChange={v => onUpdate({ capture: v })}
                    />
                    {/* Derived, not chosen: the engine follows the capture backend
                      * (docs/glossary.md, "Domain language"). It is shown because the
                      * greyed options across the form name it as the reason. */}
                    <ReadonlyField
                        label="Publish engine"
                        labelTip={engineTip(engine)}
                        value={engineValue(engine)}
                    />
                    {s.capture === "kmsgrab" && (
                        <SelectField
                            label="DRM download"
                            labelTip="How kmsgrab moves the captured scanout buffer into system memory. A tiled or compressed framebuffer needs mapping through a matching GPU device first."
                            value={s.drmMap}
                            options={DRM_MAPS}
                            onChange={v => onUpdate({ drmMap: v })}
                        />
                    )}
                    <SelectField
                        label="Monitor"
                        labelTip={withNote(
                            "Which monitor to capture. ddagrab selects the output by index; the X backends (x11grab, ximagesrc) crop the X screen to its geometry.",
                            cropNote(s.capture, selectedMonitor)
                        )}
                        value={String(s.monitor)}
                        options={monitorOptions(monitors, s.monitor)}
                        disabledReason={deps.disabled.monitor}
                        onChange={v => {
                            const index = parseInt(v, 10) || 0;
                            const hz = monitors.find(m => m.index === index)?.refreshHz ?? 0;
                            onUpdate({ monitor: index, fps: clampFps(s.fps, hz) });
                        }}
                    />
                    <NumberSelectField
                        label="Frame rate (fps)"
                        labelTip="Capture and encode frame rate, picked from the presets or typed. Higher = smoother motion, proportionally more encode load and bandwidth. Rates above the monitor's refresh rate only duplicate frames."
                        value={s.fps}
                        min={1}
                        options={fpsOptions(s.fps)}
                        optionDisabled={fpsDisabled(s.fps, selectedMonitor?.refreshHz ?? 0)}
                        onChange={fps => onUpdate({ fps })}
                    />
                    <SelectField
                        label="Audio"
                        labelTip="Audio source muxed into the stream as a second track. Viewers hear it automatically."
                        value={s.audio}
                        options={AUDIO_SOURCES}
                        optionDisabled={deps.optionDisabled.audio}
                        onChange={v => onUpdate({ audio: v })}
                    />
                </Section>

                <Section
                    icon={<IconVideo size={16} className="text-muted-foreground" />}
                    title="Encoder"
                >
                    <SelectField
                        label="Encoder family"
                        labelTip="Encoder family: the backend the encoder runs on. The software encoders (x264, x265, libvpx, three AV1), NVENC and VAAPI are wired up; a family still on the roadmap is shown greyed with the reason."
                        value={family}
                        options={familyOptions(caps)}
                        optionDisabled={deps.optionDisabled.family}
                        onChange={selectFamily}
                    />
                    <SelectField
                        label="Video codec"
                        labelTip="Video codec: the coding format the encoder produces. Efficiency and browser support follow the format (H.264, HEVC, AV1, ...); which encoder produces it follows the encoder family above."
                        value={s.codec}
                        options={codecOptions(family, caps)}
                        optionDisabled={deps.optionDisabled.codec}
                        onChange={v => onUpdate({ codec: v })}
                    />
                    <SelectField
                        label="Pixel format / chroma subsampling"
                        labelTip={withNote(
                            "Pixel format: the color model and chroma subsampling the encoder codes.",
                            deps.note.chroma
                        )}
                        value={s.chroma}
                        options={CHROMAS}
                        optionDisabled={deps.optionDisabled.chroma}
                        onChange={v => onUpdate({ chroma: v })}
                    />
                    <SelectField
                        label="Color quantization range"
                        labelTip="Quantization range for Y′CbCr code values. Ignored for gbrp - RGB is inherently full range."
                        value={s.colorRange}
                        options={RANGES}
                        disabledReason={deps.disabled.colorRange}
                        onChange={v => onUpdate({ colorRange: v })}
                    />
                    <SelectField
                        label="Encoder preset (NVENC p1-p7)"
                        labelTip={withNote(
                            "NVENC preset ladder: p1 (fastest) to p7 (most efficient compression).",
                            deps.note.encPreset
                        )}
                        value={s.encPreset}
                        options={ENC_PRESETS}
                        disabledReason={deps.disabled.encPreset}
                        onChange={v => onUpdate({ encPreset: v })}
                    />
                </Section>

                <Section
                    icon={<IconGauge size={16} className="text-muted-foreground" />}
                    title="Quality & rate control"
                >
                    <SelectField
                        label="Rate control mode"
                        labelTip="Rate-control strategy: how the encoder distributes bits over time."
                        value={s.mode}
                        options={MODES}
                        optionDisabled={deps.optionDisabled.mode}
                        onChange={v => onUpdate({ mode: v })}
                    />
                    <NumberField
                        label="Quantizer target (CQ)"
                        labelTip={withNote(cqTip(s.codec, caps), deps.note.cq)}
                        labelLink="https://en.wikipedia.org/wiki/Quantization_(image_processing)"
                        value={s.cq}
                        min={0}
                        max={cqMax(s.codec, caps)}
                        disabledReason={deps.disabled.cq}
                        onChange={v => onUpdate({ cq: v })}
                    />
                    <NumberField
                        label="Bitrate target (Mbit/s)"
                        labelTip={withNote(
                            bitrateTip(s.codec, caps),
                            deps.note.bitrateM
                        )}
                        value={s.bitrateM}
                        min={0}
                        max={bitrateLimit(s.codec, caps) || undefined}
                        disabledReason={deps.disabled.bitrateM}
                        onChange={v => onUpdate({ bitrateM: v })}
                    />
                    <NumberField
                        label="Max bitrate / ceiling (Mbit/s)"
                        labelTip={withNote(
                            "Constrained VBR only: the burst ceiling above the target. Bitrate may rise to this on motion, then fall back on static content. Set it above the bitrate target.",
                            deps.note.maxrateM
                        )}
                        value={s.maxrateM}
                        disabledReason={deps.disabled.maxrateM}
                        onChange={v => onUpdate({ maxrateM: v })}
                    />
                    <NumberField
                        label="VBV buffer (ms, 0 = auto)"
                        labelTip={withNote(
                            "Rate-control buffer for CBR and VBR, in milliseconds.\nSmaller = tighter rate and lower latency; larger = smoother quality across bursts.\n0 uses the encoder default (x264 600ms).",
                            deps.note.vbvMs
                        )}
                        labelLink="https://en.wikipedia.org/wiki/Video_buffering_verifier"
                        value={s.vbvMs}
                        disabledReason={deps.disabled.vbvMs}
                        onChange={v => onUpdate({ vbvMs: v })}
                    />
                    <NumberField
                        label="GOP length (frames, 0 = auto)"
                        labelTip={"Group of Pictures: frames between keyframes.\n0 selects auto (2×fps); any positive value is the exact keyframe interval, so 1 makes every frame a keyframe.\nNew viewers wait up to GOP/fps seconds to join; loss corrupts until the next keyframe.\nLong GOP = less bandwidth, slower joins."}
                        labelLink="https://en.wikipedia.org/wiki/Group_of_pictures"
                        value={s.gop}
                        disabledReason={deps.disabled.gop}
                        onChange={v => onUpdate({ gop: v })}
                    />
                    <NumberField
                        label="B-frames"
                        labelTip={withNote(
                            "Bi-directionally predicted frames: reference past AND future. Save bitrate in lossy modes, add reorder latency; nothing in lossless.",
                            deps.note.bframes
                        )}
                        labelLink="https://en.wikipedia.org/wiki/Group_of_pictures"
                        value={s.bframes}
                        disabledReason={deps.disabled.bframes}
                        onChange={v => onUpdate({ bframes: v })}
                    />
                </Section>

                <Section
                    icon={<IconNetwork size={16} className="text-muted-foreground" />}
                    title="Network"
                >
                    <SelectField
                        label="Publish transport protocol"
                        labelTip="How the encoded stream travels from this machine to the relay. This is the publish leg only: a viewer picks its own protocol for the watch leg (Live now, 'watch over'), so a stream published over SRT can be watched over RTSP. Protocols differ in reliability and latency control; see each option's own tooltip."
                        value={s.transport}
                        options={transportOptions}
                        optionDisabled={deps.optionDisabled.transport}
                        onChange={v => onUpdate({ transport: v })}
                    />
                    {s.transport === "srt" && (
                        <NumberField
                            label="SRT publish latency (ms, publish leg)"
                            labelTip="SRT retransmit window for the publish leg (this machine to the relay). Total glass-to-glass ≈ publish leg + watch leg + encode/decode - the windows ADD UP."
                            value={s.srtPublishLatencyMs}
                            onChange={v => onUpdate({ srtPublishLatencyMs: v })}
                        />
                    )}
                    <UplinkField
                        value={s.uplinkMbps}
                        measuring={uplink.measuring}
                        error={uplink.error}
                        onChange={v => onUpdate({ uplinkMbps: v })}
                        onRemeasure={uplink.remeasure}
                    />
                </Section>
                </fieldset>

                <div className="flex flex-wrap items-center gap-3">
                    <Button
                        onClick={onTogglePublish}
                        variant={publishing ? "destructive" : "default"}
                    >
                        {publishing && (
                            <span className="relative flex size-2">
                                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-current opacity-75" />
                                <span className="relative inline-flex size-2 rounded-full bg-current" />
                            </span>
                        )}
                        {publishing ? "Stop publishing" : "Start publishing"}
                    </Button>
                    <div className="flex items-center gap-2">
                        <Input
                            className="w-44"
                            placeholder="Preset name"
                            value={presetName}
                            onChange={e => setPresetName(e.target.value)}
                            onKeyDown={e => e.key === "Enter" && savePreset()}
                        />
                        <Button
                            variant="outline"
                            disabled={saveDisabled}
                            onClick={savePreset}
                        >
                            Save as preset
                        </Button>
                    </div>
                </div>

                <div className="flex flex-col gap-2">
                    <Badge
                        variant={webGrid.ok ? "default" : "secondary"}
                        className={VERDICT_BADGE}
                    >
                        {webGrid.ok ? (
                            <IconCheck size={14} className="shrink-0" />
                        ) : (
                            <IconX size={14} className="shrink-0" />
                        )}
                        {webGrid.text}
                    </Badge>
                    <Badge
                        variant={nativeGrid.ok ? "default" : "secondary"}
                        className={VERDICT_BADGE}
                    >
                        {nativeGrid.ok ? (
                            <IconCheck size={14} className="shrink-0" />
                        ) : (
                            <IconX size={14} className="shrink-0" />
                        )}
                        {nativeGrid.text}
                    </Badge>
                </div>

                <div className="text-xs text-muted-foreground">
                    <Tip text="Rough pre-publish estimate from resolution, fps, video codec, pixel format and rate-control mode. Real bitrate is content-dependent; live figures appear in Publish insights.">
                        <span>Expected bitrate:</span>
                    </Tip>{" "}
                    {estimate ? (
                        <>
                            <span className="text-foreground font-medium">
                                {formatEstimate(estimate)}
                            </span>
                            {" · "}
                            {resLabel}
                            {" · "}
                            {estimate.note}
                        </>
                    ) : (
                        "Resolution unavailable - the estimate needs the monitor size"
                    )}
                </div>

                <div className="space-y-1">
                    <Tip text="The exact command these settings produce.">
                        <span className="text-xs text-muted-foreground">
                            command:
                        </span>
                    </Tip>
                    <pre className="overflow-x-auto rounded-md border bg-muted/50 px-3 py-2 font-mono text-xs whitespace-pre-wrap break-all text-foreground">
                        <code>{cmd}</code>
                    </pre>
                </div>
                {pubError && (
                    <ErrorLog
                        message={pubError}
                        logPath={pubLogPath}
                        onOpenLog={onOpenLog}
                        onOpenLogsFolder={onOpenLogsFolder}
                    />
                )}
            </CardContent>
        </Card>
    );
}
