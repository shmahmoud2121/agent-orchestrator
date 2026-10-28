import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { DocumentKind } from "../lib/file-media";
import type { SheetTable } from "../lib/xlsx-reader";
import { Button } from "./ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "./ui/tabs";
import { PanelMessage, RetryButton } from "./WorkspaceDiffView";

/** Larger files open in their default app instead. */
const MAX_DOCUMENT_BYTES = 50 * 1024 * 1024;
const MAX_PDF_PAGES = 300;

type LoadState = { status: "loading" } | { status: "ready" } | { status: "error"; reason: "tooLarge" | "failed" };

async function fetchDocument(src: string, signal: AbortSignal): Promise<ArrayBuffer> {
	const response = await fetch(src, { signal });
	if (!response.ok) throw new Error(`HTTP ${response.status}`);
	const length = Number(response.headers.get("content-length"));
	if (Number.isFinite(length) && length > MAX_DOCUMENT_BYTES) throw new RangeError("too large");
	const data = await response.arrayBuffer();
	if (data.byteLength > MAX_DOCUMENT_BYTES) throw new RangeError("too large");
	return data;
}

/**
 * PDF, Word and Excel files drawn inside the app, so a document can be read
 * without leaving Altair. Everything runs in the renderer from the file's
 * bytes: no plugin, no network, no macros. Other office formats show the
 * "Open in default app" action only.
 */
export function FileDocumentPreview({ kind, path, src, onOpenExternal }: {
	kind: DocumentKind;
	path: string;
	src: string;
	onOpenExternal?: () => void;
}) {
	const { t } = useTranslation();
	const containerRef = useRef<HTMLDivElement>(null);
	const [state, setState] = useState<LoadState>({ status: "loading" });
	const [sheets, setSheets] = useState<SheetTable[]>([]);
	const [attempt, setAttempt] = useState(0);

	useEffect(() => {
		if (kind === "document") return;
		const container = containerRef.current;
		if (!container) return;
		const abort = new AbortController();
		let cancelled = false;
		setState({ status: "loading" });
		container.replaceChildren();
		void (async () => {
			try {
				const data = await fetchDocument(src, abort.signal);
				if (cancelled) return;
				if (kind === "pdf") await renderPdf(data, container, () => cancelled);
				else if (kind === "docx") await renderDocx(data, container);
				else {
					const { readWorkbook } = await import("../lib/xlsx-reader");
					const tables = await readWorkbook(data);
					if (!cancelled) setSheets(tables);
				}
				if (!cancelled) setState({ status: "ready" });
			} catch (error) {
				if (cancelled) return;
				setState({ status: "error", reason: error instanceof RangeError ? "tooLarge" : "failed" });
			}
		})();
		return () => {
			cancelled = true;
			abort.abort();
		};
	}, [kind, src, attempt]);

	const openAction = onOpenExternal ? (
		<Button onClick={onOpenExternal} size="sm" type="button" variant="outline">{t("files.document.openExternal")}</Button>
	) : undefined;

	if (kind === "document") {
		return <PanelMessage action={openAction}>{t("files.document.noPreview")}</PanelMessage>;
	}
	return (
		<div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
			{openAction ? <div className="flex shrink-0 justify-end border-b border-border px-3 py-1.5">{openAction}</div> : null}
			{state.status === "loading" ? <PanelMessage>{t("files.document.loading")}</PanelMessage> : null}
			{state.status === "error" ? (
				<PanelMessage action={<div className="flex gap-2"><RetryButton onClick={() => setAttempt((n) => n + 1)} />{openAction}</div>}>
					{t(state.reason === "tooLarge" ? "files.document.tooLarge" : "files.document.failed")}
				</PanelMessage>
			) : null}
			{kind === "xlsx" && state.status === "ready" ? <SheetTabs sheets={sheets} /> : null}
			<div
				aria-label={path}
				className={kind === "xlsx" || state.status !== "ready" ? "hidden" : "file-document-preview min-h-0 flex-1 overflow-auto bg-muted/40 p-3"}
				ref={containerRef}
			/>
		</div>
	);
}

async function renderPdf(data: ArrayBuffer, container: HTMLElement, cancelled: () => boolean): Promise<void> {
	const pdfjs = await import("pdfjs-dist");
	const { default: workerSrc } = await import("pdfjs-dist/build/pdf.worker.min.mjs?url");
	pdfjs.GlobalWorkerOptions.workerSrc = workerSrc;
	const pdf = await pdfjs.getDocument({ data, isEvalSupported: false, enableXfa: false }).promise;
	try {
		// The page area is hidden while loading, so measure the pane around it.
		const measured = container.parentElement?.clientWidth ?? 0;
		const width = Math.max(320, measured > 0 ? measured - 48 : 800);
		const ratio = window.devicePixelRatio || 1;
		for (let number = 1; number <= Math.min(pdf.numPages, MAX_PDF_PAGES); number += 1) {
			if (cancelled()) return;
			const page = await pdf.getPage(number);
			const base = page.getViewport({ scale: 1 });
			const viewport = page.getViewport({ scale: (width / base.width) * ratio });
			const canvas = document.createElement("canvas");
			canvas.width = Math.floor(viewport.width);
			canvas.height = Math.floor(viewport.height);
			canvas.style.width = `${Math.floor(viewport.width / ratio)}px`;
			canvas.className = "mx-auto mb-3 block bg-white shadow-sm";
			canvas.setAttribute("aria-label", `Page ${number}`);
			container.append(canvas);
			const context = canvas.getContext("2d");
			if (!context) continue;
			await page.render({ canvasContext: context, viewport }).promise;
		}
	} finally {
		void pdf.destroy();
	}
}

async function renderDocx(data: ArrayBuffer, container: HTMLElement): Promise<void> {
	const { renderAsync } = await import("docx-preview");
	await renderAsync(data, container, undefined, {
		inWrapper: true,
		ignoreLastRenderedPageBreak: true,
		useBase64URL: true,
		renderComments: false,
		experimental: false,
	});
	// Links inside a document must not navigate the app or run script.
	for (const link of Array.from(container.querySelectorAll("a[href]"))) {
		const href = link.getAttribute("href") ?? "";
		if (!/^(https?:|mailto:|#)/i.test(href)) link.removeAttribute("href");
		else if (!href.startsWith("#")) link.setAttribute("target", "_blank");
		link.setAttribute("rel", "noopener noreferrer");
	}
}

function SheetTabs({ sheets }: { sheets: SheetTable[] }) {
	const { t } = useTranslation();
	if (sheets.length === 0) return <PanelMessage>{t("files.document.emptyWorkbook")}</PanelMessage>;
	return (
		<Tabs className="flex min-h-0 flex-1 flex-col" defaultValue="0">
			{sheets.length > 1 ? (
				<TabsList className="mx-3 mt-2 self-start">
					{sheets.map((sheet, index) => <TabsTrigger key={index} value={String(index)}>{sheet.name}</TabsTrigger>)}
				</TabsList>
			) : null}
			{sheets.map((sheet, index) => (
				<TabsContent className="min-h-0 flex-1 overflow-auto p-3" key={index} value={String(index)}>
					{sheet.truncated ? <p className="mb-2 text-caption text-muted-foreground">{t("files.document.sheetTruncated")}</p> : null}
					<table className="border-collapse text-xs">
						<tbody>
							{sheet.rows.map((row, r) => (
								<tr key={r}>
									<th className="sticky left-0 border border-border bg-muted px-1.5 py-0.5 text-right font-normal text-muted-foreground">{r + 1}</th>
									{row.map((cell, c) => <td className="whitespace-pre border border-border px-1.5 py-0.5" dir="auto" key={c}>{cell}</td>)}
								</tr>
							))}
						</tbody>
					</table>
				</TabsContent>
			))}
		</Tabs>
	);
}
