export namespace capabilities {
	
	export class Codec {
	    name: string;
	    family: string;
	    format: string;
	    nvenc: boolean;
	    implemented: boolean;
	    chromas: string[];
	    transports: string[];
	
	    static createFrom(source: any = {}) {
	        return new Codec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.family = source["family"];
	        this.format = source["format"];
	        this.nvenc = source["nvenc"];
	        this.implemented = source["implemented"];
	        this.chromas = source["chromas"];
	        this.transports = source["transports"];
	    }
	}

}

export namespace display {
	
	export class Monitor {
	    index: number;
	    width: number;
	    height: number;
	    offsetX: number;
	    offsetY: number;
	    primary: boolean;
	    refreshHz: number;
	
	    static createFrom(source: any = {}) {
	        return new Monitor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.offsetX = source["offsetX"];
	        this.offsetY = source["offsetY"];
	        this.primary = source["primary"];
	        this.refreshHz = source["refreshHz"];
	    }
	}

}

export namespace encoders {
	
	export class Availability {
	    usable: Record<string, boolean>;
	
	    static createFrom(source: any = {}) {
	        return new Availability(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.usable = source["usable"];
	    }
	}

}

export namespace platform {
	
	export class Info {
	    os: string;
	    display: string;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.display = source["display"];
	    }
	}

}

export namespace relay {
	
	export class Path {
	    name: string;
	    ready: boolean;
	    tracks: string;
	    readers: number;
	    inMbps: number;
	
	    static createFrom(source: any = {}) {
	        return new Path(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.ready = source["ready"];
	        this.tracks = source["tracks"];
	        this.readers = source["readers"];
	        this.inMbps = source["inMbps"];
	    }
	}
	export class Status {
	    reachable: boolean;
	    error?: string;
	    paths: Path[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reachable = source["reachable"];
	        this.error = source["error"];
	        this.paths = this.convertValues(source["paths"], Path);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace settings {
	
	export class Stream {
	    name: string;
	    relayHost: string;
	    relayPort: number;
	    apiPort: number;
	    rtspPort: number;
	    webrtcPort: number;
	    transport: string;
	    codec: string;
	    mode: string;
	    chroma: string;
	    colorRange: string;
	    fps: number;
	    cq: number;
	    bitrateM: number;
	    maxrateM: number;
	    vbvMs: number;
	    gop: number;
	    bframes: number;
	    encPreset: string;
	    capture: string;
	    audio: string;
	    drmMap: string;
	    monitor: number;
	    srtPublishLatencyMs: number;
	    srtWatchLatencyMs: number;
	    uplinkMbps: number;
	
	    static createFrom(source: any = {}) {
	        return new Stream(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.relayHost = source["relayHost"];
	        this.relayPort = source["relayPort"];
	        this.apiPort = source["apiPort"];
	        this.rtspPort = source["rtspPort"];
	        this.webrtcPort = source["webrtcPort"];
	        this.transport = source["transport"];
	        this.codec = source["codec"];
	        this.mode = source["mode"];
	        this.chroma = source["chroma"];
	        this.colorRange = source["colorRange"];
	        this.fps = source["fps"];
	        this.cq = source["cq"];
	        this.bitrateM = source["bitrateM"];
	        this.maxrateM = source["maxrateM"];
	        this.vbvMs = source["vbvMs"];
	        this.gop = source["gop"];
	        this.bframes = source["bframes"];
	        this.encPreset = source["encPreset"];
	        this.capture = source["capture"];
	        this.audio = source["audio"];
	        this.drmMap = source["drmMap"];
	        this.monitor = source["monitor"];
	        this.srtPublishLatencyMs = source["srtPublishLatencyMs"];
	        this.srtWatchLatencyMs = source["srtWatchLatencyMs"];
	        this.uplinkMbps = source["uplinkMbps"];
	    }
	}
	export class Preset {
	    name: string;
	    settings: Stream;
	
	    static createFrom(source: any = {}) {
	        return new Preset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.settings = this.convertValues(source["settings"], Stream);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

