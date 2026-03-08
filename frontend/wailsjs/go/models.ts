export namespace main {
	
	export class ExportRequest {
	    backend: string;
	    outputPath: string;
	    conflictPolicy: string;
	    table: string;
	    batchSize: number;
	    compression: string;
	    schema: string;
	    host: string;
	    port: number;
	    username: string;
	    password: string;
	    database: string;
	    maxcomputeEndpoint: string;
	    maxcomputeProject: string;
	    maxcomputeAccessId: string;
	    maxcomputeAccessKey: string;
	    partitionSpec: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.backend = source["backend"];
	        this.outputPath = source["outputPath"];
	        this.conflictPolicy = source["conflictPolicy"];
	        this.table = source["table"];
	        this.batchSize = source["batchSize"];
	        this.compression = source["compression"];
	        this.schema = source["schema"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.database = source["database"];
	        this.maxcomputeEndpoint = source["maxcomputeEndpoint"];
	        this.maxcomputeProject = source["maxcomputeProject"];
	        this.maxcomputeAccessId = source["maxcomputeAccessId"];
	        this.maxcomputeAccessKey = source["maxcomputeAccessKey"];
	        this.partitionSpec = source["partitionSpec"];
	    }
	}
	export class ExportOutputCheck {
	    exists: boolean;
	    resolvedPath: string;
	    suggestedPath: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportOutputCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exists = source["exists"];
	        this.resolvedPath = source["resolvedPath"];
	        this.suggestedPath = source["suggestedPath"];
	    }
	}
	export class GeneratedFile {
	    path: string;
	    name: string;
	    size: number;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new GeneratedFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class Option {
	    value: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new Option(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	    }
	}
	export class AppConfig {
	    backends: Option[];
	    compressions: string[];
	    defaultPorts: Record<string, number>;
	    defaultState: ExportRequest;
	    backendHints: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.backends = this.convertValues(source["backends"], Option);
	        this.compressions = source["compressions"];
	        this.defaultPorts = source["defaultPorts"];
	        this.defaultState = this.convertValues(source["defaultState"], ExportRequest);
	        this.backendHints = source["backendHints"];
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
	export class ZipRequest {
	    files: string[];
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new ZipRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = source["files"];
	        this.password = source["password"];
	    }
	}
	export class ZipResult {
	    outputPath: string;
	    fileCount: number;
	
	    static createFrom(source: any = {}) {
	        return new ZipResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.outputPath = source["outputPath"];
	        this.fileCount = source["fileCount"];
	    }
	}
	

}
