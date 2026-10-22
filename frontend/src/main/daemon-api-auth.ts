export type BeforeSendHeadersDetails = {
	url: string;
	requestHeaders: Record<string, string>;
};

export type BeforeSendHeadersCallback = (response: { requestHeaders: Record<string, string> }) => void;

export type RendererSession = {
	webRequest: {
		onBeforeSendHeaders: (
			filter: { urls: string[] },
			listener: (details: BeforeSendHeadersDetails, callback: BeforeSendHeadersCallback) => void,
		) => void;
	};
};

const AUTHENTICATED_ROOT_PATHS = new Set(["/mux", "/readyz", "/shutdown"]);
const AUTHENTICATED_PREFIXES = ["/api/", "/internal/"];

export function isDaemonAuthenticatedRequest(rawURL: string, port: number | undefined): boolean {
	if (!port) return false;
	let url: URL;
	try {
		url = new URL(rawURL);
	} catch {
		return false;
	}
	if ((url.protocol !== "http:" && url.protocol !== "ws:") || url.hostname !== "127.0.0.1" || url.port !== String(port)) {
		return false;
	}
	return AUTHENTICATED_ROOT_PATHS.has(url.pathname) || AUTHENTICATED_PREFIXES.some((prefix) => url.pathname.startsWith(prefix));
}

/**
 * Installs the daemon credential only on the trusted shell renderer session.
 * Callers must never pass an in-app browser, agent-viewer, preview, webview, or
 * render-check session here.
 */
export function installDaemonAPIAuthentication(options: {
	session: RendererSession;
	port: () => number | undefined;
	readToken: () => string | null;
}): void {
	options.session.webRequest.onBeforeSendHeaders(
		{ urls: ["http://127.0.0.1:*/*", "ws://127.0.0.1:*/*"] },
		(details, callback) => {
			const token = isDaemonAuthenticatedRequest(details.url, options.port()) ? options.readToken() : null;
			callback({
				requestHeaders: token
					? { ...details.requestHeaders, Authorization: `Bearer ${token}` }
					: details.requestHeaders,
			});
		},
	);
}
