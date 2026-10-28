import type { ChatSkill } from "../../types/conversation";

/**
 * `/delegate` in a manager chat. It never reaches the agent as a slash command
 * (a harness would reject an unknown one); it becomes a plain instruction the
 * manager's standing rules recognise.
 */
const DELEGATE = /^\/delegate(?:\s+|$)/i;

export const DELEGATE_PREFIX = "Hand this to workers, don't do it yourself.";

export function expandDelegateCommand(text: string): string {
	const match = DELEGATE.exec(text);
	if (!match) return text;
	const request = text.slice(match[0].length).trim();
	return request
		? `${DELEGATE_PREFIX} Write each worker a complete brief from our conversation.\n\n${request}`
		: `${DELEGATE_PREFIX} Take my last request and write each worker a complete brief from our conversation.`;
}

export function withDelegateCommand(manager: boolean, skills: ChatSkill[]): ChatSkill[] {
	if (!manager) return skills;
	return [
		{ name: "delegate", displayName: "delegate", description: "Hand this request to workers", source: "AO" },
		...skills.filter((skill) => skill.name !== "delegate"),
	];
}
