import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { THEMES } from "./theme-palettes";
import { buildThemeStylesheet, deriveThemeTokens } from "./theme-tokens";

const tokensCss = readFileSync(resolve(__dirname, "../../styles/tokens.css"), "utf8");

describe("theme tokens", () => {
	it("only derives tokens that tokens.css defines", () => {
		const unknown = Object.keys(deriveThemeTokens(THEMES[1].dark, "dark")).filter(
			(name) => !tokensCss.includes(`${name}:`),
		);
		expect(unknown).toEqual([]);
	});

	it("emits a dark and a light block for every named theme, never for Orchestrate", () => {
		const css = buildThemeStylesheet();
		for (const theme of THEMES.slice(1)) {
			expect(css).toContain(`:root[data-style-theme="${theme.id}"] {`);
			expect(css).toContain(`:root[data-style-theme="${theme.id}"][data-theme="light"] {`);
		}
		expect(css).not.toContain('"orchestrate"');
	});

	it("resolves every derived value to a concrete hex color for xterm", () => {
		for (const theme of THEMES) {
			for (const mode of ["dark", "light"] as const) {
				for (const value of Object.values(deriveThemeTokens(theme[mode], mode))) {
					expect(value).toMatch(/^#[0-9a-f]{6}([0-9a-f]{2})?$/);
				}
			}
		}
	});
});

const toLinear = (channel: number) => (channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4);
const fromLinear = (channel: number) => (channel <= 0.0031308 ? channel * 12.92 : 1.055 * channel ** (1 / 2.4) - 0.055);

function oklabToLinear([L, a, b]: [number, number, number]): [number, number, number] {
	const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
	const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
	const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
	return [
		4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
		-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
		-0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
	];
}

function inGamut(rgb: [number, number, number]): boolean {
	return rgb.every((channel) => channel >= -1e-4 && channel <= 1 + 1e-4);
}

/** CSS Color 4 oklch, gamut-mapped by reducing chroma, as linear sRGB. */
function oklchToLinear(input: string): [number, number, number] {
	const match = input.match(/oklch\(\s*([\d.]+)\s+([\d.]+)\s+([\d.]+)/);
	if (!match) throw new Error(`not oklch: ${input}`);
	const L = Number(match[1]);
	const C = Number(match[2]);
	const hue = (Number(match[3]) * Math.PI) / 180;
	let lab: [number, number, number] = [L, C * Math.cos(hue), C * Math.sin(hue)];
	if (!inGamut(oklabToLinear(lab))) {
		let lo = 0;
		let hi = C;
		const angle = Math.atan2(lab[2], lab[1]);
		for (let step = 0; step < 32; step += 1) {
			const mid = (lo + hi) / 2;
			const candidate: [number, number, number] = [L, mid * Math.cos(angle), mid * Math.sin(angle)];
			if (inGamut(oklabToLinear(candidate))) lo = mid;
			else hi = mid;
		}
		lab = [L, lo * Math.cos(angle), lo * Math.sin(angle)];
	}
	return oklabToLinear(lab).map((channel) => toLinear(fromLinear(Math.min(1, Math.max(0, channel))))) as [
		number,
		number,
		number,
	];
}

function hexToLinear(hex: string): [number, number, number] {
	const value = hex.replace("#", "");
	return [0, 2, 4].map((index) => toLinear(Number.parseInt(value.slice(index, index + 2), 16) / 255)) as [
		number,
		number,
		number,
	];
}

function colorToLinear(input: string): [number, number, number] {
	return input.startsWith("#") ? hexToLinear(input) : oklchToLinear(input);
}

function contrast(foreground: string, background: string): number {
	const luminance = (rgb: [number, number, number]) => 0.2126 * rgb[0] + 0.7152 * rgb[1] + 0.0722 * rgb[2];
	const [hi, lo] = [luminance(colorToLinear(foreground)), luminance(colorToLinear(background))].sort((a, b) => b - a);
	return (hi + 0.05) / (lo + 0.05);
}

function cssBlock(css: string, header: string): string {
	const start = css.indexOf(header);
	if (start < 0) throw new Error(header);
	const open = css.indexOf("{", start);
	let depth = 0;
	for (let index = open; index < css.length; index += 1) {
		if (css[index] === "{") depth += 1;
		else if (css[index] === "}") {
			depth -= 1;
			if (depth === 0) return css.slice(open + 1, index);
		}
	}
	throw new Error(`unclosed ${header}`);
}

function cssToken(block: string, name: string): string {
	const match = block.match(new RegExp(`(?:^|\\n)\\t${name}:\\s*([^;]+);`));
	if (!match) throw new Error(name);
	return match[1].trim();
}

function resolveToken(block: string, name: string, seen = new Set<string>()): string {
	const raw = cssToken(block, name);
	const reference = raw.match(/^var\((--[\w-]+)\)$/);
	if (!reference) return raw;
	if (seen.has(reference[1])) throw new Error(`cycle at ${name}`);
	seen.add(reference[1]);
	return resolveToken(block, reference[1], seen);
}

describe("default theme contrast", () => {
	const normalized = tokensCss.replace(/\r\n/g, "\n");
	const dark = cssBlock(normalized, ":root,\n:root.dark,\n.dark {");
	const light = cssBlock(normalized, '\n:root[data-theme="light"] {');

	it("keeps the Observatory token values", () => {
		expect(resolveToken(dark, "--background")).toBe("#050506");
		expect(resolveToken(dark, "--foreground")).toBe("#ECE7DC");
		expect(resolveToken(dark, "--accent-star")).toBe("#E5B451");
		expect(resolveToken(dark, "--ring")).toBe("#E5B451");
		expect(resolveToken(dark, "--sidebar-ring")).toBe("#E5B451");
		expect(resolveToken(light, "--background")).toBe("#F4F0E6");
		expect(resolveToken(light, "--card")).toBe("#FBF8F1");
		expect(resolveToken(light, "--accent-star")).toBe("#D9A441");
		expect(resolveToken(light, "--accent-star-ink")).toBe("#8A5E0E");
		expect(resolveToken(light, "--ring")).toBe("#8A5E0E");
		expect(cssToken(dark, "--border")).toBe("#1E1C18");
		expect(cssToken(light, "--border")).toBe("#D8D0BE");
	});

	it("clears the text and gold floors on both themes", () => {
		for (const [label, block] of [
			["dark", dark],
			["light", light],
		] as const) {
			const foreground = resolveToken(block, "--foreground");
			const muted = resolveToken(block, "--muted-foreground");
			const goldInk = resolveToken(block, "--accent-star-ink");
			const ring = resolveToken(block, "--ring");
			const surfaces = ["--background", "--card", "--popover", "--sidebar"] as const;
			for (const surface of surfaces) {
				const ratio = contrast(foreground, resolveToken(block, surface));
				expect(ratio, `${label} foreground on ${surface} = ${ratio.toFixed(2)}`).toBeGreaterThanOrEqual(7);
			}
			for (const surface of ["--background", "--card", "--muted"] as const) {
				const ratio = contrast(muted, resolveToken(block, surface));
				expect(ratio, `${label} muted on ${surface} = ${ratio.toFixed(2)}`).toBeGreaterThanOrEqual(4.5);
			}
			// Gold used as text or a rule must read as text; the focus ring needs 3:1.
			for (const surface of ["--background", "--card"] as const) {
				const inkRatio = contrast(goldInk, resolveToken(block, surface));
				expect(inkRatio, `${label} gold ink on ${surface} = ${inkRatio.toFixed(2)}`).toBeGreaterThanOrEqual(4.5);
				const ringRatio = contrast(ring, resolveToken(block, surface));
				expect(ringRatio, `${label} ring on ${surface} = ${ringRatio.toFixed(2)}`).toBeGreaterThanOrEqual(3);
			}
		}
	});

	it("keeps default terminal ANSI text readable on both plates", () => {
		const visible = ["red", "green", "yellow", "blue", "magenta", "cyan", "white", "bright-black", "bright-red", "bright-green", "bright-yellow", "bright-blue", "bright-magenta", "bright-cyan", "bright-white"] as const;
		for (const slot of visible) {
			const ratio = contrast(cssToken(dark, `--color-term-${slot}`), resolveToken(dark, "--background"));
			expect(ratio, `dark ${slot} = ${ratio.toFixed(2)}`).toBeGreaterThanOrEqual(4.5);
		}
		for (const slot of visible) {
			const ratio = contrast(cssToken(light, `--color-term-${slot}`), resolveToken(light, "--background"));
			expect(ratio, `light ${slot} = ${ratio.toFixed(2)}`).toBeGreaterThanOrEqual(4.5);
		}
		const dim = contrast(cssToken(light, "--color-text-terminal-dim"), resolveToken(light, "--background"));
		expect(dim, `light terminal dim = ${dim.toFixed(2)}`).toBeGreaterThanOrEqual(4.5);
	});

	it("keeps light status signals at 3:1 on the paper surfaces", () => {
		for (const name of ["--color-status-working", "--color-status-needs-you", "--color-status-validating", "--color-status-in-review", "--color-status-ready", "--color-status-merged"] as const) {
			for (const surface of ["--background", "--card"] as const) {
				const ratio = contrast(cssToken(light, name), resolveToken(light, surface));
				expect(ratio, `light ${name} on ${surface} = ${ratio.toFixed(2)}`).toBeGreaterThanOrEqual(3);
			}
		}
	});
});
