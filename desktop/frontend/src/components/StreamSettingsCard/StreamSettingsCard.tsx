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
import { BrowserVerdict, Deps, Monitor, Option, Stream } from "../../types/stream";
import {
    AUDIO_SOURCES, CAPTURES, CHROMAS, DRM_MAPS, ENC_PRESETS, MODES,
    RANGES, TRANSPORT_META, clampFps, codecOptions, familyOptions, fpsDisabled,
    fpsOptions, monitorOptions,
} from "../../util/options";
import { Capability, familyOf } from "../../util/domain";
import { estimateBitrate, formatEstimate } from "../../util/estimate";
import Tip from "../Tip/Tip";
import ErrorLog from "../ErrorLog/ErrorLog";
import SelectField from "../fields/SelectField";
import NumberField from "../fields/NumberField";
import TextField from "../fields/TextField";
import UplinkField from "../fields/UplinkField";

/** Auto-fitting field grid shared by every settings section. */
const GRID =
    "grid gap-x-5 gap-y-3 [grid-template-columns:repeat(auto-fit,minmax(230px,1fr))]";

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
    monitors: Monitor[];
    browser: BrowserVerdict;
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

/** The full stream-settings form: dependency-aware fields, publish controls,
 * browser-viewability verdict and the live command preview. */
export default function StreamSettingsCard({
    s,
    deps,
    caps,
    transports,
    monitors,
    browser,
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
    const estimate = estimateBitrate(
        s,
        selectedMonitor?.width ?? 0,
        selectedMonitor?.height ?? 0,
        caps
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
                    {/* Each transport exposes only its own relay listener port:
                     * a backend implementation knob is hidden while its
                     * transport is not selected (docs/field-availability.md). */}
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
                            labelTip="TCP port of the relay's WebRTC/WHIP listener (default 8889)."
                            value={s.webrtcPort}
                            onChange={v => onUpdate({ webrtcPort: v })}
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
                        label="Capture API"
                        labelTip="Screen capture API feeding the encoder."
                        value={s.capture}
                        options={CAPTURES}
                        optionDisabled={deps.optionDisabled.capture}
                        onChange={v => onUpdate({ capture: v })}
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
                        labelTip="Which monitor to capture. ddagrab selects the output by index; x11grab crops the X screen to it."
                        value={String(s.monitor)}
                        options={monitorOptions(monitors, s.monitor)}
                        disabledReason={deps.disabled.monitor}
                        onChange={v => {
                            const index = parseInt(v, 10) || 0;
                            const hz = monitors.find(m => m.index === index)?.refreshHz ?? 0;
                            onUpdate({ monitor: index, fps: clampFps(s.fps, hz) });
                        }}
                    />
                    <SelectField
                        label="Frame rate (fps)"
                        labelTip="Capture and encode frame rate. Higher = smoother motion, proportionally more encode load and bandwidth. Rates above the monitor's refresh rate only duplicate frames."
                        value={String(s.fps)}
                        options={fpsOptions(s.fps)}
                        optionDisabled={fpsDisabled(s.fps, selectedMonitor?.refreshHz ?? 0)}
                        onChange={v => onUpdate({ fps: parseInt(v, 10) || 0 })}
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
                        label="Encoder"
                        labelTip="Encoder backend. Software x264 and NVIDIA NVENC are wired up; the other hardware families (VAAPI, QSV, AMF, V4L2, Rockchip MPP, Vulkan) are on the roadmap and shown greyed until implemented."
                        value={family}
                        options={familyOptions(caps)}
                        optionDisabled={deps.optionDisabled.family}
                        onChange={selectFamily}
                    />
                    <SelectField
                        label="Video codec"
                        labelTip="Video coding format produced by the selected encoder. Efficiency and browser support follow the format (H.264, HEVC, AV1, ...); the encoder backend follows the family above."
                        value={s.codec}
                        options={codecOptions(family, caps)}
                        optionDisabled={deps.optionDisabled.codec}
                        onChange={v => onUpdate({ codec: v })}
                    />
                    <SelectField
                        label="Pixel format / chroma subsampling"
                        labelTip="Pixel format: the color model and chroma subsampling the encoder codes."
                        value={s.chroma}
                        options={CHROMAS}
                        optionDisabled={deps.optionDisabled.chroma}
                        disabledReason={deps.disabled.chroma}
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
                        labelTip="NVENC preset ladder: p1 (fastest) to p7 (most efficient compression)."
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
                        labelTip="Constant quantizer for quality mode - the CRF value on x264, the QP target on NVENC. Lower = better + more bits. 12 ≈ visually lossless, 19 ≈ excellent, 28 ≈ visibly compressed."
                        value={s.cq}
                        disabledReason={deps.disabled.cq}
                        onChange={v => onUpdate({ cq: v })}
                    />
                    <NumberField
                        label="Bitrate target (Mbit/s)"
                        labelTip="Target rate for CBR (held constant), VBR and ABR (averaged toward). CRF and lossless set no bitrate."
                        value={s.bitrateM}
                        disabledReason={deps.disabled.bitrateM}
                        onChange={v => onUpdate({ bitrateM: v })}
                    />
                    <NumberField
                        label="Max bitrate / ceiling (Mbit/s)"
                        labelTip="Constrained VBR only: the burst ceiling above the target. Bitrate may rise to this on motion, then fall back on static content. Set it above the bitrate target."
                        value={s.maxrateM}
                        disabledReason={deps.disabled.maxrateM}
                        onChange={v => onUpdate({ maxrateM: v })}
                    />
                    <NumberField
                        label="VBV buffer (ms, 0 = auto)"
                        labelTip={"Rate-control buffer for CBR and VBR, in milliseconds.\nSmaller = tighter rate and lower latency; larger = smoother quality across bursts.\n0 uses the encoder default (x264 600ms)."}
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
                        onChange={v => onUpdate({ gop: v })}
                    />
                    <NumberField
                        label="B-frames"
                        labelTip="Bi-directionally predicted frames: reference past AND future. Save bitrate in lossy modes, add reorder latency; nothing in lossless."
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
                        label="Transport protocol"
                        labelTip="How the encoded stream is carried across the network from publisher to relay to viewer. Protocols differ in reliability, latency control and which players can receive them; pick one, then see each option's own tooltip."
                        value={s.transport}
                        options={transportOptions}
                        optionDisabled={deps.optionDisabled.transport}
                        onChange={v => onUpdate({ transport: v })}
                    />
                    {s.transport === "srt" && (
                        <NumberField
                            label="SRT publish latency (ms, hop 1)"
                            labelTip="SRT retransmit window for YOUR hop (publisher to relay). Total glass-to-glass ≈ hop 1 + hop 2 + encode/decode - the windows ADD UP."
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

                <Badge
                    variant={browser.ok ? "default" : "secondary"}
                    className="whitespace-normal text-left"
                >
                    {browser.ok ? (
                        <IconCheck size={14} className="shrink-0" />
                    ) : (
                        <IconX size={14} className="shrink-0" />
                    )}
                    {browser.text}
                </Badge>

                <div className="text-xs text-muted-foreground">
                    <Tip text="Rough pre-publish estimate from resolution, fps, codec, chroma and rate control. Real bitrate is content-dependent; live figures appear in Publish insights.">
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
                        "resolution unavailable - estimate needs the monitor size"
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
