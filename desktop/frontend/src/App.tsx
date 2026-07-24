import { useState } from "react";
import {
    IconAppWindow,
    IconFlask,
    IconLayoutGrid,
    IconLayoutGridFilled,
} from "@tabler/icons-react";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";
import { usePlatform } from "./hooks/usePlatform";
import { useEncoders } from "./hooks/useEncoders";
import { useCapabilities } from "./hooks/useCapabilities";
import { useStreamSettings } from "./hooks/useStreamSettings";
import { usePublish } from "./hooks/usePublish";
import { useLive } from "./hooks/useLive";
import { useWall } from "./hooks/useWall";
import { useGridViewer } from "./hooks/useGridViewer";
import { useTestPublishers } from "./hooks/useTestPublishers";
import { useUplinkMeasure } from "./hooks/useUplinkMeasure";
import { useMonitors } from "./hooks/useMonitors";
import { useLogs } from "./hooks/useLogs";
import LoadingScreen from "./components/LoadingScreen/LoadingScreen";
import PresetCard from "./components/PresetCard/PresetCard";
import StreamSettingsCard from "./components/StreamSettingsCard/StreamSettingsCard";
import PublishInsightsCard from "./components/PublishInsightsCard/PublishInsightsCard";
import LiveNowCard from "./components/LiveNowCard/LiveNowCard";
import StreamGridPage from "./components/StreamGridPage/StreamGridPage";

/** Test streams published per toggle: fills a near-square grid row and leaves
 * room to compare against a real capture alongside. */
const TEST_STREAM_COUNT = 3;

/**
 * Composition root: wires the state hooks to the presentational cards. All
 * logic lives in hooks/ and util/; this file only distributes it.
 */
export default function App() {
    const platform = usePlatform();
    const encoders = useEncoders();
    const capabilities = useCapabilities();
    const settings = useStreamSettings(platform, encoders, capabilities);
    const publish = usePublish(settings.s);
    const live = useLive();
    const wall = useWall();
    const gridViewer = useGridViewer();
    const testPub = useTestPublishers();
    const uplink = useUplinkMeasure(settings.update);
    const monitors = useMonitors();
    const logs = useLogs();
    const [gridOpen, setGridOpen] = useState(false);

    if (!settings.s || !settings.deps || !settings.browser) {
        return <LoadingScreen />;
    }

    // The native grid decodes with GStreamer, so it plays every stream the
    // relay serves; it opens on all ready streams over RTSP (TCP, retransmits).
    const wallStreams = (live.live?.paths ?? [])
        .filter(p => p.ready)
        .map(p => p.name);
    const wallTransport = live.watchTransports.includes("rtsp")
        ? "rtsp"
        : live.watchTransports[0];

    return (
        <TooltipProvider>
            <div className="p-4 space-y-4 max-w-7xl mx-auto">
                <div className="flex items-center justify-between">
                    <h1 className="text-xl font-semibold">screen-sharing</h1>
                    <div className="flex items-center gap-2">
                        <Button
                            variant="outline"
                            size="sm"
                            onClick={() =>
                                testPub.count > 0
                                    ? void testPub.stop()
                                    : void testPub.start(TEST_STREAM_COUNT)
                            }
                        >
                            <IconFlask size={16} />
                            {testPub.count > 0
                                ? "Stop test streams"
                                : "Test streams"}
                        </Button>
                        <Button
                            variant="outline"
                            size="sm"
                            onClick={() => setGridOpen(true)}
                        >
                            <IconLayoutGrid size={16} /> Grid view
                        </Button>
                        <Button
                            variant="outline"
                            size="sm"
                            disabled={
                                !gridViewer.running &&
                                (wallStreams.length === 0 || !wallTransport)
                            }
                            onClick={() =>
                                gridViewer.running
                                    ? void gridViewer.stop()
                                    : void gridViewer.start(
                                          wallStreams,
                                          wallTransport,
                                      )
                            }
                        >
                            <IconLayoutGridFilled size={16} />
                            {gridViewer.running
                                ? "Close GTK grid"
                                : "GTK grid"}
                        </Button>
                        <Button
                            variant="outline"
                            size="sm"
                            disabled={
                                !wall.running &&
                                (wallStreams.length === 0 || !wallTransport)
                            }
                            onClick={() =>
                                wall.running
                                    ? void wall.stop()
                                    : void wall.start(wallStreams, wallTransport)
                            }
                        >
                            <IconAppWindow size={16} />
                            {wall.running ? "Close native grid" : "Native grid"}
                        </Button>
                    </div>
                </div>
                {wall.error && (
                    <p className="text-sm text-destructive whitespace-pre-wrap">
                        {wall.error}
                    </p>
                )}
                {gridViewer.error && (
                    <p className="text-sm text-destructive whitespace-pre-wrap">
                        {gridViewer.error}
                    </p>
                )}
                {testPub.error && (
                    <p className="text-sm text-destructive whitespace-pre-wrap">
                        {testPub.error}
                    </p>
                )}

                <PresetCard
                    preset={settings.preset}
                    userPresets={settings.userPresets}
                    publishing={publish.publishing}
                    onApplyPreset={settings.applyPreset}
                    onDeletePreset={settings.deletePreset}
                />

                <StreamSettingsCard
                    s={settings.s}
                    deps={settings.deps}
                    caps={capabilities}
                    transports={settings.transports}
                    monitors={monitors}
                    browser={settings.browser}
                    cmd={settings.cmd}
                    publishing={publish.publishing}
                    pubError={publish.error}
                    pubLogPath={publish.logPath}
                    uplink={uplink}
                    onUpdate={settings.update}
                    onTogglePublish={publish.toggle}
                    onSavePreset={settings.saveAsPreset}
                    onOpenLog={logs.openLog}
                    onOpenLogsFolder={logs.openLogsFolder}
                />

                <PublishInsightsCard
                    stats={publish.stats}
                    avg5={publish.avg5}
                    peak={publish.peak}
                    targetFps={settings.s.fps}
                    uplinkMbps={settings.s.uplinkMbps}
                    publishing={publish.publishing}
                />

                <LiveNowCard
                    live={live.live}
                    watching={live.watching}
                    watchTransports={live.watchTransports}
                    connecting={live.connecting}
                    error={live.error}
                    logPath={live.logPath}
                    watchLatencyMs={settings.s.srtWatchLatencyMs}
                    onToggleWatch={live.toggleWatch}
                    onUpdateWatchLatency={v =>
                        settings.update({ srtWatchLatencyMs: v })
                    }
                    onOpenLog={logs.openLog}
                    onOpenLogsFolder={logs.openLogsFolder}
                />

                {gridOpen && (
                    <StreamGridPage
                        paths={live.live?.paths ?? []}
                        s={settings.s}
                        onClose={() => setGridOpen(false)}
                    />
                )}
            </div>
        </TooltipProvider>
    );
}
