export namespace activity {
	
	export class Entry {
	    time: string;
	    provider: string;
	    alias: string;
	    model?: string;
	    status: string;
	    ok: boolean;
	    durationMs: number;
	    message: string;
	    stream?: boolean;
	    queueWaitMs?: number;
	    execMs?: number;
	    ttftMs?: number;
	    errCategory?: string;
	    cancelReason?: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.provider = source["provider"];
	        this.alias = source["alias"];
	        this.model = source["model"];
	        this.status = source["status"];
	        this.ok = source["ok"];
	        this.durationMs = source["durationMs"];
	        this.message = source["message"];
	        this.stream = source["stream"];
	        this.queueWaitMs = source["queueWaitMs"];
	        this.execMs = source["execMs"];
	        this.ttftMs = source["ttftMs"];
	        this.errCategory = source["errCategory"];
	        this.cancelReason = source["cancelReason"];
	    }
	}

}

export namespace core {
	
	export class ModelInput {
	    id: string;
	    provider: string;
	    displayName: string;
	    upstreamModel: string;
	    streamMode: string;
	    timeoutSeconds: number;
	    enabled: boolean;
	    systemPrompt: string;
	    temperature?: number;
	    maxTokens?: number;
	    contextWindow?: number;
	    extraArgs: string[];
	    workingDir: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.provider = source["provider"];
	        this.displayName = source["displayName"];
	        this.upstreamModel = source["upstreamModel"];
	        this.streamMode = source["streamMode"];
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.enabled = source["enabled"];
	        this.systemPrompt = source["systemPrompt"];
	        this.temperature = source["temperature"];
	        this.maxTokens = source["maxTokens"];
	        this.contextWindow = source["contextWindow"];
	        this.extraArgs = source["extraArgs"];
	        this.workingDir = source["workingDir"];
	    }
	}
	export class ModelView {
	    id: string;
	    provider: string;
	    providerName: string;
	    displayName: string;
	    upstreamModel?: string;
	    streamMode: string;
	    timeoutSeconds: number;
	    enabled: boolean;
	    status: string;
	    statusKind: string;
	    ready: boolean;
	    capabilities?: provider.Capabilities;
	
	    static createFrom(source: any = {}) {
	        return new ModelView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.provider = source["provider"];
	        this.providerName = source["providerName"];
	        this.displayName = source["displayName"];
	        this.upstreamModel = source["upstreamModel"];
	        this.streamMode = source["streamMode"];
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.enabled = source["enabled"];
	        this.status = source["status"];
	        this.statusKind = source["statusKind"];
	        this.ready = source["ready"];
	        this.capabilities = this.convertValues(source["capabilities"], provider.Capabilities);
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
	    capabilities: provider.Capabilities;
	    queueDepth?: number;
	
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
	        this.capabilities = this.convertValues(source["capabilities"], provider.Capabilities);
	        this.queueDepth = source["queueDepth"];
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
	    host: string;
	    port: number;
	    url: string;
	    configUrl: string;
	    requireApiKey: boolean;
	    activeRequests: number;
	    providers: ProviderView[];
	    models: ModelView[];
	    activity: activity.Entry[];
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverRunning = source["serverRunning"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.url = source["url"];
	        this.configUrl = source["configUrl"];
	        this.requireApiKey = source["requireApiKey"];
	        this.activeRequests = source["activeRequests"];
	        this.providers = this.convertValues(source["providers"], ProviderView);
	        this.models = this.convertValues(source["models"], ModelView);
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

export namespace provider {
	
	export class Capabilities {
	    streaming: boolean;
	    tools: boolean;
	    structured_output: boolean;
	    usage: boolean;
	    vision: boolean;
	    model_selection: boolean;
	    sessions: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Capabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.streaming = source["streaming"];
	        this.tools = source["tools"];
	        this.structured_output = source["structured_output"];
	        this.usage = source["usage"];
	        this.vision = source["vision"];
	        this.model_selection = source["model_selection"];
	        this.sessions = source["sessions"];
	    }
	}

}

