import { useCallback, useEffect, useRef, useState } from "react";
import { WhepConn } from "./useWhep";

/** Names offered as synthetic test streams in the grid roster. */
export const TEST_STREAM_NAMES = [
    "test-1",
    "test-2",
    "test-3",
    "test-4",
    "test-5",
    "test-6",
];

/** Simulated handshake delay so the connecting state is visible. */
const CONNECT_DELAY_MS = 600;

interface TestSource {
    stream: MediaStream;
    stop: () => void;
}

/** Builds one synthetic source: a canvas animation (bouncing dot, name, clock)
 * captured at 30 fps, plus a quiet per-stream sine tone so mute and volume
 * controls have something audible to act on. */
function startSource(name: string, index: number, ctx: AudioContext): TestSource {
    const canvas = document.createElement("canvas");
    canvas.width = 640;
    canvas.height = 360;
    const g = canvas.getContext("2d")!;
    const hue = (index * 57) % 360;
    const t0 = performance.now();
    let raf = 0;

    const draw = () => {
        const t = (performance.now() - t0) / 1000;
        g.fillStyle = `hsl(${hue} 40% 18%)`;
        g.fillRect(0, 0, 640, 360);
        const x = 320 + 260 * Math.sin(t * (0.7 + index * 0.13));
        const y = 180 + 130 * Math.sin(t * (1.1 + index * 0.07));
        g.fillStyle = `hsl(${hue} 80% 60%)`;
        g.beginPath();
        g.arc(x, y, 24, 0, Math.PI * 2);
        g.fill();
        g.fillStyle = "white";
        g.font = "28px sans-serif";
        g.fillText(name, 16, 40);
        g.font = "20px monospace";
        g.fillText(t.toFixed(1) + " s", 16, 70);
        raf = requestAnimationFrame(draw);
    };
    draw();

    const stream = canvas.captureStream(30);

    const osc = ctx.createOscillator();
    osc.frequency.value = 220 + index * 110;
    const gain = ctx.createGain();
    gain.gain.value = 0.05;
    const dest = ctx.createMediaStreamDestination();
    osc.connect(gain).connect(dest);
    osc.start();
    for (const track of dest.stream.getAudioTracks()) stream.addTrack(track);

    return {
        stream,
        stop: () => {
            cancelAnimationFrame(raf);
            osc.stop();
            for (const track of stream.getTracks()) track.stop();
        },
    };
}

/**
 * Locally generated test streams with the same connection interface as
 * useWhep, so grid tiles treat them like relay streams. No network or WebRTC
 * involved: video comes from a canvas capture, audio from an oscillator.
 * Useful for exercising layout and controls without any publisher.
 */
export function useTestStreams() {
    const [conns, setConns] = useState<Record<string, WhepConn>>({});
    const sources = useRef(new Map<string, TestSource>());
    const timers = useRef(new Map<string, number>());
    const audio = useRef<AudioContext | null>(null);

    const connect = useCallback((name: string) => {
        if (sources.current.has(name)) return;
        // Created on click, so the user gesture satisfies autoplay policy.
        audio.current ??= new AudioContext();
        void audio.current.resume();

        const index = Math.max(TEST_STREAM_NAMES.indexOf(name), 0);
        const src = startSource(name, index, audio.current);
        sources.current.set(name, src);
        setConns(prev => ({
            ...prev,
            [name]: { state: "connecting", stream: src.stream },
        }));

        const timer = window.setTimeout(() => {
            timers.current.delete(name);
            setConns(prev =>
                prev[name]
                    ? { ...prev, [name]: { state: "connected", stream: src.stream } }
                    : prev
            );
        }, CONNECT_DELAY_MS);
        timers.current.set(name, timer);
    }, []);

    const disconnect = useCallback((name: string) => {
        const timer = timers.current.get(name);
        if (timer) clearTimeout(timer);
        timers.current.delete(name);
        sources.current.get(name)?.stop();
        sources.current.delete(name);
        setConns(prev => {
            const next = { ...prev };
            delete next[name];
            return next;
        });
    }, []);

    useEffect(() => {
        const s = sources.current;
        const t = timers.current;
        const a = audio;
        return () => {
            for (const timer of t.values()) clearTimeout(timer);
            t.clear();
            for (const src of s.values()) src.stop();
            s.clear();
            void a.current?.close();
            a.current = null;
        };
    }, []);

    return { conns, connect, disconnect };
}
