export namespace display {
	
	export class Monitor {
	    index: number;
	    width: number;
	    height: number;
	    primary: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Monitor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.primary = source["primary"];
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
	    transport: string;
	    codec: string;
	    mode: string;
	    chroma: string;
	    colorRange: string;
	    fps: number;
	    cq: number;
	    bitrateM: number;
	    gop: number;
	    bframes: number;
	    encPreset: string;
	    capture: string;
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
	        this.transport = source["transport"];
	        this.codec = source["codec"];
	        this.mode = source["mode"];
	        this.chroma = source["chroma"];
	        this.colorRange = source["colorRange"];
	        this.fps = source["fps"];
	        this.cq = source["cq"];
	        this.bitrateM = source["bitrateM"];
	        this.gop = source["gop"];
	        this.bframes = source["bframes"];
	        this.encPreset = source["encPreset"];
	        this.capture = source["capture"];
	        this.monitor = source["monitor"];
	        this.srtPublishLatencyMs = source["srtPublishLatencyMs"];
	        this.srtWatchLatencyMs = source["srtWatchLatencyMs"];
	        this.uplinkMbps = source["uplinkMbps"];
	    }
	}

}

