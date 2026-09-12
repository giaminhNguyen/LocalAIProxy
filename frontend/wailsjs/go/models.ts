export namespace activity {
	
	export class Entry {
	    time: string;
	    provider: string;
	    alias: string;
	    status: string;
	    ok: boolean;
	    durationMs: number;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.provider = source["provider"];
	        this.alias = source["alias"];
	        this.status = source["status"];
	        this.ok = source["ok"];
	        this.durationMs = source["durationMs"];
	        this.message = source["message"];
	    }
	}

}

export namespace core {
	
	export class TestResult {
	    passed: boolean;
	    latencyMs: number;
	    response?: string;
	    message?: string;
	    detail?: string;
	    time: string;
	
	    static createFrom(source: any = {}) {
	        return new TestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.passed = source["passed"];
	        this.latencyMs = source["latencyMs"];
	        this.response = source["response"];
	        this.message = source["message"];
	        this.detail = source["detail"];
	        this.time = source["time"];
	    }
	}
	export class ProviderView {
	    alias: string;
	    name: string;
	    enabled: boolean;
	    installed: boolean;
	    version: string;
	    executable: string;
	    auth: string;
	    status: string;
	    statusKind: string;
	    ready: boolean;
	    concurrency: number;
	    maxQueue: number;
	    queueTimeoutSec: number;
	    execTimeoutSec: number;
	    lastTest?: TestResult;
	    installedBy: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.alias = source["alias"];
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.installed = source["installed"];
	        this.version = source["version"];
	        this.executable = source["executable"];
	        this.auth = source["auth"];
	        this.status = source["status"];
	        this.statusKind = source["statusKind"];
	        this.ready = source["ready"];
	        this.concurrency = source["concurrency"];
	        this.maxQueue = source["maxQueue"];
	        this.queueTimeoutSec = source["queueTimeoutSec"];
	        this.execTimeoutSec = source["execTimeoutSec"];
	        this.lastTest = this.convertValues(source["lastTest"], TestResult);
	        this.installedBy = source["installedBy"];
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
	export class Snapshot {
	    serverRunning: boolean;
	    port: number;
	    url: string;
	    configUrl: string;
	    providers: ProviderView[];
	    activity: activity.Entry[];
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverRunning = source["serverRunning"];
	        this.port = source["port"];
	        this.url = source["url"];
	        this.configUrl = source["configUrl"];
	        this.providers = this.convertValues(source["providers"], ProviderView);
	        this.activity = this.convertValues(source["activity"], activity.Entry);
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

