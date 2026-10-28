import { describe, expect, it } from "vitest";
import { DELEGATE_PREFIX, expandDelegateCommand, withDelegateCommand } from "./delegate-command";

describe("expandDelegateCommand", () => {
	it("turns /delegate into a plain instruction", () => {
		const out = expandDelegateCommand("/delegate graph the fiscal ratios");
		expect(out.startsWith(DELEGATE_PREFIX)).toBe(true);
		expect(out.endsWith("graph the fiscal ratios")).toBe(true);
	});

	it("points a bare /delegate at the last request", () => {
		expect(expandDelegateCommand("/delegate")).toContain("my last request");
	});

	it("leaves other text alone", () => {
		expect(expandDelegateCommand("/delegated is a word")).toBe("/delegated is a word");
		expect(expandDelegateCommand("please /delegate this")).toBe("please /delegate this");
	});
});

describe("withDelegateCommand", () => {
	it("offers /delegate only to managers", () => {
		expect(withDelegateCommand(true, []).map((s) => s.name)).toEqual(["delegate"]);
		expect(withDelegateCommand(false, [])).toEqual([]);
	});
});
