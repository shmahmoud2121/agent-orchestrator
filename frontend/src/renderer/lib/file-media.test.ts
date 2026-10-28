import { describe, expect, it } from "vitest";
import { fileMediaKind, isDocumentKind } from "./file-media";

describe("fileMediaKind", () => {
	it.each([
		["Output/ratios.pdf", "pdf"],
		["memo.DOCX", "docx"],
		["figures.xlsx", "xlsx"],
		["macro.xlsm", "xlsx"],
		["deck.pptx", "document"],
		["old.doc", "document"],
		["chart.png", "image"],
		["notes.md", undefined],
	])("%s -> %s", (path, kind) => {
		expect(fileMediaKind(path)).toBe(kind);
	});

	it("tells documents from other media", () => {
		expect(isDocumentKind("pdf")).toBe(true);
		expect(isDocumentKind("image")).toBe(false);
	});
});
