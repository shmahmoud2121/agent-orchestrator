// @vitest-environment node
import { describe, expect, it } from "vitest";
import { contentSecurityPolicy } from "./vite.renderer.config";

describe("packaged renderer CSP", () => {
	it.each(["build", "serve"] as const)("allows media only from the app and loopback preview origins in %s", (mode) => {
		expect(contentSecurityPolicy(mode).split("; ").filter((directive) => directive.startsWith("media-src")))
			.toEqual(["media-src 'self' http://127.0.0.1:* http://*.localhost:*"]);
	});

	it.each(["build", "serve"] as const)("allows connections only to the app and loopback in %s (local-only)", (mode) => {
		const connectSrc = contentSecurityPolicy(mode)
			.split("; ")
			.filter((directive) => directive.startsWith("connect-src"));
		const expected = ["connect-src", "'self'", "http://127.0.0.1:*", "ws://127.0.0.1:*", "http://*.localhost:*"];
		if (mode === "serve") expected.push("ws://localhost:*");
		expect(connectSrc).toEqual([expected.join(" ")]);
	});

	it.each(["build", "serve"] as const)("does not load external images automatically in %s", (mode) => {
		expect(contentSecurityPolicy(mode).split("; ").filter((directive) => directive.startsWith("img-src")))
			.toEqual(["img-src 'self' data: http://127.0.0.1:* http://*.localhost:*"]);
	});

	it("frames agent renders from the loopback daemon and nothing else", () => {
		const frameSrc = contentSecurityPolicy("build")
			.split("; ")
			.filter((directive) => directive.startsWith("frame-src"));
		expect(frameSrc).toEqual(["frame-src 'self' http://127.0.0.1:* http://*.localhost:*"]);
	});
});
