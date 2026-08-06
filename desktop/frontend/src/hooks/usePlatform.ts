import { useEffect, useState } from "react";
import { Platform } from "../../wailsjs/go/app/App";
import { PlatformInfo } from "../types/stream";

/**
 * Detects the OS and (on Linux) the display server once from the backend. Null
 * until the query resolves; dependency logic treats null as "unknown" and does
 * not restrict capture backends.
 */
export function usePlatform(): PlatformInfo | null {
    const [platform, setPlatform] = useState<PlatformInfo | null>(null);

    useEffect(() => {
        Platform()
            .then(setPlatform)
            .catch(() => setPlatform(null));
    }, []);

    return platform;
}
