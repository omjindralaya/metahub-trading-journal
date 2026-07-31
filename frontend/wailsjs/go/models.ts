export namespace backend {
	
	export class CloudProfile {
	    username: string;
	    email: string;
	    display_name: string;
	    tier: string;
	    is_connected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CloudProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.username = source["username"];
	        this.email = source["email"];
	        this.display_name = source["display_name"];
	        this.tier = source["tier"];
	        this.is_connected = source["is_connected"];
	    }
	}
	export class EntitlementView {
	    can_sync_now: boolean;
	    tier: string;
	    effective_tier: string;
	    expired: boolean;
	    stale: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EntitlementView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.can_sync_now = source["can_sync_now"];
	        this.tier = source["tier"];
	        this.effective_tier = source["effective_tier"];
	        this.expired = source["expired"];
	        this.stale = source["stale"];
	    }
	}
	export class MT5AccountInfo {
	    login: number;
	    server: string;
	    currency: string;
	    balance: number;
	    account_type: string;
	    connected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MT5AccountInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.login = source["login"];
	        this.server = source["server"];
	        this.currency = source["currency"];
	        this.balance = source["balance"];
	        this.account_type = source["account_type"];
	        this.connected = source["connected"];
	    }
	}
	export class OpenPosition {
	    ticket: string;
	    symbol: string;
	    type: string;
	    volume: number;
	    open_price: number;
	    current_price: number;
	    sl: number;
	    tp: number;
	    floating_pnl: number;
	    account_type: string;
	    mt5_login: string;
	    mt5_server: string;
	
	    static createFrom(source: any = {}) {
	        return new OpenPosition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ticket = source["ticket"];
	        this.symbol = source["symbol"];
	        this.type = source["type"];
	        this.volume = source["volume"];
	        this.open_price = source["open_price"];
	        this.current_price = source["current_price"];
	        this.sl = source["sl"];
	        this.tp = source["tp"];
	        this.floating_pnl = source["floating_pnl"];
	        this.account_type = source["account_type"];
	        this.mt5_login = source["mt5_login"];
	        this.mt5_server = source["mt5_server"];
	    }
	}
	export class SyncTargetInfo {
	    login: string;
	    server: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncTargetInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.login = source["login"];
	        this.server = source["server"];
	    }
	}
	export class Trade {
	    id: number;
	    ticket: string;
	    // Go type: time
	    open_time: any;
	    // Go type: time
	    close_time: any;
	    symbol: string;
	    type: string;
	    volume: number;
	    open_price: number;
	    close_price: number;
	    sl: number;
	    tp: number;
	    commission: number;
	    swap: number;
	    profit: number;
	    net_profit: number;
	    position_id: string;
	    account_type: string;
	    mt5_login: string;
	    mt5_server: string;
	    // Go type: time
	    cloud_synced_at?: any;
	    // Go type: time
	    cloud_blocked_at?: any;
	
	    static createFrom(source: any = {}) {
	        return new Trade(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.ticket = source["ticket"];
	        this.open_time = this.convertValues(source["open_time"], null);
	        this.close_time = this.convertValues(source["close_time"], null);
	        this.symbol = source["symbol"];
	        this.type = source["type"];
	        this.volume = source["volume"];
	        this.open_price = source["open_price"];
	        this.close_price = source["close_price"];
	        this.sl = source["sl"];
	        this.tp = source["tp"];
	        this.commission = source["commission"];
	        this.swap = source["swap"];
	        this.profit = source["profit"];
	        this.net_profit = source["net_profit"];
	        this.position_id = source["position_id"];
	        this.account_type = source["account_type"];
	        this.mt5_login = source["mt5_login"];
	        this.mt5_server = source["mt5_server"];
	        this.cloud_synced_at = this.convertValues(source["cloud_synced_at"], null);
	        this.cloud_blocked_at = this.convertValues(source["cloud_blocked_at"], null);
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

