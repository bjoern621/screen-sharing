import { AudioControl, AudioSnapshot } from "../../types/sink";

/**
 * Audio controls backed by an HTMLMediaElement. Holds the desired mute/volume
 * and applies them to the element. Also carries the autoplay-muted fallback:
 * when playback with sound is blocked, forceMuted() flips to muted so the sink
 * can retry play(), and the UI reflects the auto-mute.
 *
 * Raising the volume above zero expresses intent to hear the stream, so it also
 * unmutes.
 */
export class ElementAudioControl implements AudioControl {
    private muted = false;
    private volume = 1;
    private snapshot: AudioSnapshot = { muted: false, volume: 1 };

    constructor(
        private el: HTMLMediaElement,
        private onChange: () => void
    ) {
        this.apply();
    }

    setMuted(muted: boolean): void {
        if (muted === this.muted) return;
        this.muted = muted;
        this.commit();
    }

    setVolume(volume: number): void {
        const unmute = volume > 0 && this.muted;
        if (volume === this.volume && !unmute) return;
        this.volume = volume;
        if (unmute) this.muted = false;
        this.commit();
    }

    getSnapshot(): AudioSnapshot {
        return this.snapshot;
    }

    /** Flip to muted after an autoplay-with-sound rejection. */
    forceMuted(): void {
        if (this.muted) return;
        this.muted = true;
        this.commit();
    }

    private apply(): void {
        this.el.muted = this.muted;
        this.el.volume = this.volume;
    }

    private commit(): void {
        this.apply();
        this.snapshot = { muted: this.muted, volume: this.volume };
        this.onChange();
    }
}
