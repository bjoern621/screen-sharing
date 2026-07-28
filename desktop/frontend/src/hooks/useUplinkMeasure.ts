import { useCallback, useState } from "react";
import { MeasureUplink } from "../../wailsjs/go/main/App";
import { Stream } from "../types/stream";

/**
 * Runs the backend upload speed test and writes the measured Mbit/s into the
 * uplink field via the caller's settings updater. Exposes the loading and error
 * state so the button can show progress.
 *
 * The field the figure lands in is what every bitrate warning is judged against,
 * so a measurement it cannot hold is reported as an error rather than rounded
 * into it.
 */
export function useUplinkMeasure(update: (patch: Partial<Stream>) => void) {
    const [measuring, setMeasuring] = useState(false);
    const [error, setError] = useState("");

    const remeasure = useCallback(async () => {
        setError("");
        setMeasuring(true);
        try {
            const value = await MeasureUplink();
            const rounded = Math.round(value);
            if (rounded < 1) {
                setError(
                    `measured ${value.toFixed(1)} Mbps, below the 1 Mbps the uplink field holds`
                );
                return;
            }
            update({ uplinkMbps: rounded });
        } catch (e) {
            setError("measure failed: " + e);
        } finally {
            setMeasuring(false);
        }
    }, [update]);

    return { measuring, error, remeasure };
}
