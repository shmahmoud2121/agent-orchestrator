// @vitest-environment jsdom
import JSZip from "jszip";
import { describe, expect, it } from "vitest";
import { columnIndex, readWorkbook } from "./xlsx-reader";

const NS = 'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"';

async function workbook(): Promise<ArrayBuffer> {
	const zip = new JSZip();
	zip.file("xl/workbook.xml", `<workbook ${NS}><sheets><sheet name="Ratios" sheetId="1" r:id="rId1"/><sheet name="Notes" sheetId="2" r:id="rId2"/></sheets></workbook>`);
	zip.file("xl/_rels/workbook.xml.rels", '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Target="/xl/worksheets/sheet2.xml"/></Relationships>');
	zip.file("xl/sharedStrings.xml", `<sst ${NS}><si><t>Current ratio</t></si><si><r><t>نسبة </t></r><r><t>السيولة</t></r></si></sst>`);
	zip.file("xl/worksheets/sheet1.xml", `<worksheet ${NS}><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="C1"><v>1.85</v></c></row><row r="3"><c r="B3" t="s"><v>1</v></c><c r="D3" t="b"><v>1</v></c></row></sheetData></worksheet>`);
	zip.file("xl/worksheets/sheet2.xml", `<worksheet ${NS}><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>draft</t></is></c><c r="B1"><f>SUM(1,2)</f><v>3</v></c></row></sheetData></worksheet>`);
	return zip.generateAsync({ type: "arraybuffer" });
}

describe("readWorkbook", () => {
	it("reads values, shared and inline strings, gaps and every sheet", async () => {
		const sheets = await readWorkbook(await workbook());
		expect(sheets.map((s) => s.name)).toEqual(["Ratios", "Notes"]);
		expect(sheets[0]!.rows).toEqual([["Current ratio", "", "1.85"], [], ["", "نسبة السيولة", "", "TRUE"]]);
		expect(sheets[1]!.rows).toEqual([["draft", "3"]]);
	});

	it("maps column letters", () => {
		expect(columnIndex("A1")).toBe(0);
		expect(columnIndex("Z9")).toBe(25);
		expect(columnIndex("AA10")).toBe(26);
		expect(columnIndex("BC12")).toBe(54);
	});
});
