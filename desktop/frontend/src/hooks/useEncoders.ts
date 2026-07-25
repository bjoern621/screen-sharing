import { useEffect, useState } from "react";
import { Encoders } from "../../wailsjs/go/main/App";
import { EncoderInfo } from "../types/stream";

/**
 * Probes once which video encoders this machine can run, per publish engine. Null
 * until the probe resolves; dependency logic treats null as "unknown" and does not
 * restrict codecs, so the UI never greys out a codec before the answer is known.
 */
export function useEncoders(): EncoderInfo | null {
    const [encoders, setEncoders] = useState<EncoderInfo | null>(null);

    useEffect(() => {
        Encoders()
            .then(setEncoders)
            .catch(() => setEncoders(null));
    }, []);

    return encoders;
}
