import { COMPANY } from "./constants";

/** Shows legacy product names in page metadata as the current product name. */
export function visibleProductName(text: string): string {
  return text
    .replaceAll("Agent Orchestrator", COMPANY.NAME)
    .replace(/\bOrchestrator\b/g, COMPANY.NAME)
    .replace(/\bAO\b(?! Cloud)/g, COMPANY.NAME);
}
