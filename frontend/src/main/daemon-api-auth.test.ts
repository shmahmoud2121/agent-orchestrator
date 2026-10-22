import { describe, expect, it, vi } from "vitest";
import {
	installDaemonAPIAuthentication,
	type BeforeSendHeadersCallback,
	type BeforeSendHeadersDetails,
} from "./daemon-api-auth";

function fakeSession() {
	let listener: ((details: BeforeSendHeadersDetails, callback: BeforeSendHeadersCallback) => void) | undefined;
	return {
		session: {
			webRequest: {
				onBeforeSendHeaders: vi.fn((_filter, next) => { listener = next; }),
			},
		},
		request(url: string): Record<string, string> {
			let headers: Record<string, string> = { Existing: "yes" };
			listener?.({ url, requestHeaders: headers }, (result) => { headers = result.requestHeaders; });
			return headers;
		},
	};
}

describe("daemon API authentication", () => {
	it("adds the token only to API, event, WebSocket, and control requests on the exact daemon origin", () => {
		const renderer = fakeSession();
		installDaemonAPIAuthentication({ session: renderer.session, port: () => 4317, readToken: () => "secret" });

		for (const url of [
			"http://127.0.0.1:4317/api/v1/sessions",
			"http://127.0.0.1:4317/api/v1/events?after=latest",
			"ws://127.0.0.1:4317/mux",
			"http://127.0.0.1:4317/internal/telemetry/desktop",
			"http://127.0.0.1:4317/readyz",
			"http://127.0.0.1:4317/shutdown",
		]) {
			expect(renderer.request(url).Authorization, url).toBe("Bearer secret");
		}

		for (const url of [
			"http://127.0.0.1:4317/healthz",
			"http://127.0.0.1:4318/api/v1/sessions",
			"http://localhost:4317/api/v1/sessions",
			"http://ao-preview.mfrc2.localhost:4317/index.html",
			"https://example.com/api/v1/sessions",
		]) {
			expect(renderer.request(url).Authorization, url).toBeUndefined();
		}
	});

	it("never installs a listener on browser, preview, agent-viewer, or render-check sessions", () => {
		const renderer = fakeSession();
		const browser = fakeSession();
		const preview = fakeSession();
		const agentViewer = fakeSession();
		const renderCheck = fakeSession();
		installDaemonAPIAuthentication({ session: renderer.session, port: () => 3001, readToken: () => "secret" });

		expect(renderer.session.webRequest.onBeforeSendHeaders).toHaveBeenCalledOnce();
		for (const untrusted of [browser, preview, agentViewer, renderCheck]) {
			expect(untrusted.session.webRequest.onBeforeSendHeaders).not.toHaveBeenCalled();
			expect(untrusted.request("http://127.0.0.1:3001/api/v1/sessions").Authorization).toBeUndefined();
		}
	});
});
