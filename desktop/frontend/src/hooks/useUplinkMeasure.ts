import { useCallback, useState } from "react";
import { MeasureUplink } from "../../wailsjs/go/main/App";
import { Stream } from "../types/stream";

/**
 * Runs the backend upload speed test and writes the measured Mbit/s into the
 * uplink field via the caller's settings updater. Exposes the loading and error
 * state so the button can show progress.
 */
export function useUplinkMeasure(update: (patch: Partial<Stream>) => void) {
    const [measuring, setMeasuring] = useState(false);
    const [error, setError] = useState("");

    const remeasure = useCallback(async () => {
        setError("");
        setMeasuring(true);
        try {
            const value = await MeasureUplink();
            update({ uplinkMbps: Math.max(1, Math.round(value)) });
        } catch (e) {
            setError("measure failed: " + e);
        } finally {
            setMeasuring(false);
        }
    }, [update]);

    return { measuring, error, remeasure };
}
