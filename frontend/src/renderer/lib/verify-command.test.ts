import { describe, expect, it } from "vitest";
import { formatVerifyCommand, parseVerifyCommand, parseVerifyTimeout } from "./verify-command";

describe("verify command line", () => {
	it("splits on whitespace and honours quotes and escapes", () => {
		expect(parseVerifyCommand("  go test ./... ")).toEqual(["go", "test", "./..."]);
		expect(parseVerifyCommand(`npm run "test all" -- --grep 'a b'`)).toEqual(["npm", "run", "test all", "--", "--grep", "a b"]);
		expect(parseVerifyCommand(`echo "say \\"hi\\"" it\\'s ''`)).toEqual(["echo", 'say "hi"', "it's", ""]);
		expect(parseVerifyCommand("")).toEqual([]);
	});

	it("never interprets shell syntax", () => {
		expect(parseVerifyCommand("make test && rm -rf $HOME")).toEqual(["make", "test", "&&", "rm", "-rf", "$HOME"]);
	});

	it("rejects an unterminated quote", () => {
		expect(parseVerifyCommand(`go test "./...`)).toBeUndefined();
		expect(parseVerifyCommand("it's")).toBeUndefined();
	});

	it("round-trips any argument list", () => {
		for (const args of [["go", "test", "./..."], ["sh", "-c", "make test && make lint"], ["it's", "", `a "b"`, "tab\there"]]) {
			expect(parseVerifyCommand(formatVerifyCommand(args))).toEqual(args);
		}
		expect(formatVerifyCommand(undefined)).toBe("");
	});
});

describe("verify timeout", () => {
	it("accepts whole seconds within the cap and blank for the default", () => {
		expect(parseVerifyTimeout(" ", 7200)).toBeUndefined();
		expect(parseVerifyTimeout("900", 7200)).toBe(900);
		for (const bad of ["0", "7201", "1.5", "-3", "ten"]) expect(parseVerifyTimeout(bad, 7200)).toBeNaN();
	});
});
