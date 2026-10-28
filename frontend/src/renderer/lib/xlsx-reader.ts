import JSZip from "jszip";

export type SheetTable = { name: string; rows: string[][]; truncated: boolean };

const MAX_ROWS = 1000;
const MAX_COLUMNS = 100;
const MAIN_NS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main";
const REL_NS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships";

function xml(text: string): Document {
	return new DOMParser().parseFromString(text, "application/xml");
}

function children(node: ParentNode, name: string): Element[] {
	return Array.from(node.childNodes).filter((child): child is Element => child.nodeType === 1 && (child as Element).localName === name);
}

function all(node: Document | Element, name: string): Element[] {
	return Array.from(node.getElementsByTagNameNS(MAIN_NS, name));
}

/** "BC12" -> 54 (zero-based column index). */
export function columnIndex(ref: string): number {
	let index = 0;
	for (const char of ref.replace(/\d+$/, "").toUpperCase()) index = index * 26 + (char.charCodeAt(0) - 64);
	return index - 1;
}

function textOf(node: Element): string {
	// Rich text keeps its runs under <r><t>; plain text is a single <t>.
	return all(node, "t").map((t) => t.textContent ?? "").join("");
}

/**
 * Reads the cell values of an .xlsx workbook: shared and inline strings,
 * numbers and booleans as written. Formulas show their cached result. No
 * formatting, which keeps this small and safe to run on untrusted files.
 */
export async function readWorkbook(data: ArrayBuffer): Promise<SheetTable[]> {
	const zip = await JSZip.loadAsync(data);
	const read = async (name: string) => (await zip.file(name)?.async("string")) ?? "";
	const shared = all(xml(await read("xl/sharedStrings.xml")), "si").map(textOf);
	const workbook = xml(await read("xl/workbook.xml"));
	const rels = xml(await read("xl/_rels/workbook.xml.rels"));
	const targets = new Map<string, string>();
	for (const rel of Array.from(rels.getElementsByTagName("Relationship"))) {
		targets.set(rel.getAttribute("Id") ?? "", rel.getAttribute("Target") ?? "");
	}
	const tables: SheetTable[] = [];
	for (const sheet of all(workbook, "sheet")) {
		const target = targets.get(sheet.getAttributeNS(REL_NS, "id") ?? "") ?? "";
		const file = target.startsWith("/") ? target.slice(1) : `xl/${target.replace(/^\.\//, "")}`;
		const doc = xml(await read(file));
		const rows: string[][] = [];
		let truncated = false;
		for (const row of all(doc, "row")) {
			const rowNumber = Number(row.getAttribute("r")) || rows.length + 1;
			if (rowNumber > MAX_ROWS) {
				truncated = true;
				break;
			}
			const cells: string[] = [];
			for (const cell of children(row, "c")) {
				const col = cell.getAttribute("r") ? columnIndex(cell.getAttribute("r")!) : cells.length;
				if (col >= MAX_COLUMNS) {
					truncated = true;
					continue;
				}
				const type = cell.getAttribute("t");
				const value = children(cell, "v")[0]?.textContent ?? "";
				let text = value;
				if (type === "s") text = shared[Number(value)] ?? "";
				else if (type === "inlineStr") text = children(cell, "is")[0] ? textOf(children(cell, "is")[0]!) : "";
				else if (type === "b") text = value === "1" ? "TRUE" : "FALSE";
				while (cells.length < col) cells.push("");
				cells[col] = text;
			}
			while (rows.length < rowNumber - 1) rows.push([]);
			rows[rowNumber - 1] = cells;
		}
		tables.push({ name: sheet.getAttribute("name") ?? `Sheet ${tables.length + 1}`, rows, truncated });
	}
	return tables;
}
