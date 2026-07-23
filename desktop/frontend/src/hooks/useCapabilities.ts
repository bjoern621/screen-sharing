import { useEffect, useState } from "react";
import { Capabilities } from "../../wailsjs/go/main/App";
import { Capability } from "../util/domain";

/**
 * Fetches the backend codec capability table once. Null until it resolves;
 * dependency logic treats null as "unknown" and imposes no codec constraint, so
 * options are never greyed out before the facts arrive.
 */
export function useCapabilities(): Capability[] | null {
    const [caps, setCaps] = useState<Capability[] | null>(null);

    useEffect(() => {
        Capabilities()
            .then(setCaps)
            .catch(() => setCaps(null));
    }, []);

    return caps;
}
