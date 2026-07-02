export namespace config {
	
	export class Config {
	    server_url: string;
	    logsheet_id: string;
	    token: string;
	    n1mm_enabled: boolean;
	    n1mm_port: number;
	    jtdx_enabled: boolean;
	    jtdx_port: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server_url = source["server_url"];
	        this.logsheet_id = source["logsheet_id"];
	        this.token = source["token"];
	        this.n1mm_enabled = source["n1mm_enabled"];
	        this.n1mm_port = source["n1mm_port"];
	        this.jtdx_enabled = source["jtdx_enabled"];
	        this.jtdx_port = source["jtdx_port"];
	    }
	}

}

export namespace main {
	
	export class ActivityEntry {
	    time: string;
	    level: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ActivityEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.level = source["level"];
	        this.message = source["message"];
	    }
	}

}

