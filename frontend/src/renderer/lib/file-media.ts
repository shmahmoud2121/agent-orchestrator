import { isRasterImagePath } from "./image-files";

export type FileMediaKind = "image" | "video" | "audio" | DocumentKind;

/** Documents previewed in the app; "document" ones only open in their default app. */
export type DocumentKind = "pdf" | "docx" | "xlsx" | "document";

/** Common media containers; decoding failures stay in the viewer. */
export function fileMediaKind(path: string): FileMediaKind | undefined {
	if (isRasterImagePath(path)) return "image";
	if (/\.(m4v|mov|mp4|ogv|webm)$/i.test(path)) return "video";
	if (/\.(aac|flac|m4a|mp3|oga|ogg|opus|wav)$/i.test(path)) return "audio";
	return documentKind(path);
}

export function documentKind(path: string): DocumentKind | undefined {
	if (/\.pdf$/i.test(path)) return "pdf";
	if (/\.docx$/i.test(path)) return "docx";
	if (/\.xls[xm]$/i.test(path)) return "xlsx";
	if (/\.(doc|xls|ppt|pptx|odt|ods|odp|rtf)$/i.test(path)) return "document";
	return undefined;
}

export function isDocumentKind(kind: FileMediaKind | undefined): kind is DocumentKind {
	return kind === "pdf" || kind === "docx" || kind === "xlsx" || kind === "document";
}
