import { useEffect, useState } from "react";
import { Decoders } from "../../wailsjs/go/app/App";
import { Decoder } from "../util/domain";

/**
 * Fetches the backend decode capability table once. Null until it resolves, and the
 * form says nothing about decoding until then rather than guessing: the note it feeds
 * explains a choice and blocks nothing, so its absence costs a sentence and no
 * correctness.
 */
export function useDecoders(): Decoder[] | null {
    const [decoders, setDecoders] = useState<Decoder[] | null>(null);

    useEffect(() => {
        Decoders()
            .then(setDecoders)
            .catch(() => setDecoders(null));
    }, []);

    return decoders;
}
