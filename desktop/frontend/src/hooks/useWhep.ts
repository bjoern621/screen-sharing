import { useCallback, useEffect, useRef, useState } from "react";

/** Connection lifecycle of one WHEP subscription. */
export type WhepState = "connecting" | "connected" | "failed";

/** One WHEP subscription: lifecycle state, the received tracks, and the
 * failure reason when state is "failed". The MediaStream object is stable for
 * the life of the subscription; tracks are added to it as they arrive. */
export interface WhepConn {
    state: WhepState;
    stream: MediaStream;
    error?: string;
}

/** Resolves once ICE gathering finishes, or after a short timeout so a stuck
 * gatherer cannot block the offer. Host candidates on a LAN arrive well within
 * the timeout. */
function iceComplete(pc: RTCPeerConnection): Promise<void> {
    if (pc.iceGatheringState === "complete") return Promise.resolve();
    return new Promise(resolve => {
        const timer = setTimeout(done, 1000);
        function done() {
            clearTimeout(timer);
            pc.removeEventListener("icegatheringstatechange", check);
            resolve();
        }
        function check() {
            if (pc.iceGatheringState === "complete") done();
        }
        pc.addEventListener("icegatheringstatechange", check);
    });
}

/**
 * Manages WHEP (RFC 9725) subscriptions to the relay, one RTCPeerConnection
 * per stream name. connect() POSTs a receive-only SDP offer to the relay's
 * /whep endpoint and collects the answered tracks into a MediaStream.
 * disconnect() DELETEs the session resource and closes the peer connection.
 * All sessions close when the owning component unmounts.
 *
 * The relay re-serves every ingested stream over WHEP regardless of the
 * publish transport, but the browser only decodes H.264 video and Opus audio.
 */
export function useWhep(relayHost: string, webrtcPort: number) {
    const [conns, setConns] = useState<Record<string, WhepConn>>({});
    const sessions = useRef(
        new Map<string, { pc: RTCPeerConnection; resource?: string }>()
    );

    const connect = useCallback(
        async (name: string) => {
            if (sessions.current.has(name)) return;

            const pc = new RTCPeerConnection();
            const stream = new MediaStream();
            sessions.current.set(name, { pc });
            setConns(prev => ({ ...prev, [name]: { state: "connecting", stream } }));

            const fail = (error: string) => {
                // Only the session that still owns this name may report failure;
                // a disconnect() or a newer connect() has taken over otherwise.
                if (sessions.current.get(name)?.pc !== pc) return;
                sessions.current.delete(name);
                pc.close();
                setConns(prev => ({
                    ...prev,
                    [name]: { state: "failed", stream, error },
                }));
            };

            pc.addTransceiver("video", { direction: "recvonly" });
            pc.addTransceiver("audio", { direction: "recvonly" });
            pc.ontrack = e => stream.addTrack(e.track);
            pc.onconnectionstatechange = () => {
                const cs = pc.connectionState;
                if (cs === "connected") {
                    if (sessions.current.get(name)?.pc !== pc) return;
                    setConns(prev => ({
                        ...prev,
                        [name]: { state: "connected", stream },
                    }));
                } else if (cs === "failed" || cs === "disconnected") {
                    fail(`connection ${cs}`);
                }
            };

            const endpoint = `http://${relayHost}:${webrtcPort}/${encodeURIComponent(name)}/whep`;
            try {
                await pc.setLocalDescription(await pc.createOffer());
                await iceComplete(pc);
                const res = await fetch(endpoint, {
                    method: "POST",
                    headers: { "Content-Type": "application/sdp" },
                    body: pc.localDescription!.sdp,
                });
                if (!res.ok) {
                    throw new Error(`WHEP POST ${res.status} ${res.statusText}`);
                }
                const entry = sessions.current.get(name);
                if (!entry || entry.pc !== pc) return; // disconnected mid-handshake
                const loc = res.headers.get("Location");
                if (loc) entry.resource = new URL(loc, endpoint).toString();
                await pc.setRemoteDescription({
                    type: "answer",
                    sdp: await res.text(),
                });
            } catch (e) {
                fail(String(e));
            }
        },
        [relayHost, webrtcPort]
    );

    const disconnect = useCallback((name: string) => {
        const entry = sessions.current.get(name);
        sessions.current.delete(name);
        if (entry) {
            if (entry.resource) {
                void fetch(entry.resource, { method: "DELETE" }).catch(() => {});
            }
            entry.pc.close();
        }
        setConns(prev => {
            const next = { ...prev };
            delete next[name];
            return next;
        });
    }, []);

    useEffect(() => {
        const s = sessions.current;
        return () => {
            for (const entry of s.values()) {
                if (entry.resource) {
                    void fetch(entry.resource, { method: "DELETE" }).catch(() => {});
                }
                entry.pc.close();
            }
            s.clear();
        };
    }, []);

    return { conns, connect, disconnect };
}
