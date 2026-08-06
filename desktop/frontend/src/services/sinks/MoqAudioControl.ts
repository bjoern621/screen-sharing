import { AudioControl, AudioSnapshot } from "../../types/sink";
import { MediaMTXMoQReader } from "./vendor/mediamtxMoqReader";

/**
 * Audio controls backed by a Media-over-QUIC reader's AudioContext, the
 * counterpart of ElementAudioControl for the path with no media element.
 *
 * Muted is the AudioContext's suspension rather than a flag of its own, which is
 * also the state it starts in: a context built without a user gesture is
 * suspended, so a MoQ tile arrives silent and says so. The reader reports every
 * such transition, including the ones it makes on its own across a reconnect, and
 * reportMuted is where they land - so the control follows the reader rather than
 * keeping a second opinion about whether sound is playing.
 *
 * Raising the volume above zero expresses intent to hear the stream, so it also
 * unmutes, matching ElementAudioControl.
 */
export class MoqAudioControl implements AudioControl {
    private muted = true;
    private volume = 1;
    private snapshot: AudioSnapshot = { muted: true, volume: 1 };

    /**
     * The reader is supplied through a getter because the sink builds it
     * asynchronously, after the tile already holds this control: a call arriving
     * before the subscription is held here and applied when the reader appears.
     */
    constructor(
        private reader: () => MediaMTXMoQReader | undefined,
        private onChange: () => void
    ) {}

    setMuted(muted: boolean): void {
        if (muted === this.muted) return;
        this.muted = muted;
        this.reader()?.setMuted(muted);
        this.commit();
    }

    setVolume(volume: number): void {
        const unmute = volume > 0 && this.muted;
        if (volume === this.volume && !unmute) return;
        this.volume = volume;
        this.reader()?.setVolume(volume);
        if (unmute) {
            this.muted = false;
            this.reader()?.setMuted(false);
        }
        this.commit();
    }

    getSnapshot(): AudioSnapshot {
        return this.snapshot;
    }

    /**
     * Record a mute transition the reader made. Called from its onAudioMuted, so
     * an AudioContext that came back suspended after a reconnect is reflected
     * rather than contradicted.
     */
    reportMuted(muted: boolean): void {
        if (muted === this.muted) return;
        this.muted = muted;
        this.commit();
    }

    /** Push the held volume into a reader that has just been built. */
    applyTo(reader: MediaMTXMoQReader): void {
        reader.setVolume(this.volume);
        if (!this.muted) reader.setMuted(false);
    }

    private commit(): void {
        this.snapshot = { muted: this.muted, volume: this.volume };
        this.onChange();
    }
}
