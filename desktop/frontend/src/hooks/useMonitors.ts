import { useEffect, useState } from "react";
import { Monitors } from "../../wailsjs/go/main/App";
import { Monitor } from "../types/stream";

/**
 * Lists the display monitors (index, resolution, primary flag), queried once
 * from the backend. Empty until the query resolves; components fall back to the
 * saved monitor index when the list is empty.
 */
export function useMonitors(): Monitor[] {
    const [monitors, setMonitors] = useState<Monitor[]>([]);

    useEffect(() => {
        Monitors()
            .then(setMonitors)
            .catch(() => setMonitors([]));
    }, []);

    return monitors;
}
