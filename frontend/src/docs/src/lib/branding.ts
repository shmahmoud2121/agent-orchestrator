export function visibleProductName(text: string): string {
  return text.replaceAll("Agent Orchestrator", "Altair").replace(/\bAO\b/g, "Altair");
}
