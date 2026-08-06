import { MoqCert } from "../../wailsjs/go/app/App";
import { MoqCert as Cert } from "../services/sinks/MoqSink";

/**
 * Reads the relay's Media-over-QUIC certificate fingerprint through the backend.
 *
 * The fetch cannot happen in the page: WebTransport refuses a plain listener, the
 * relay's certificate is self-signed by default, and the app's own origin has
 * never accepted it, so the request the relay's own reader page makes fails here.
 * Go makes it instead and the page pins what comes back.
 *
 * A thin wrapper over the one binding the sink layer needs, on the rule that keeps
 * bindings out of components and services (frontend-coding-style.md, "Layers").
 */
export function moqCert(streamName: string): Promise<Cert> {
    return MoqCert(streamName);
}
