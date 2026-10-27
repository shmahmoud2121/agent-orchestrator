import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { agentReadinessQueryKey } from "../hooks/useAgentReadinessQuery";
import { agentReadiness } from "../test/agent-readiness-fixtures";
import { CreateWorkProjectSheet } from "./CreateWorkProjectSheet";
import { TooltipProvider } from "./ui/tooltip";

function renderSheet() {
	const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
	queryClient.setQueryData(agentReadinessQueryKey, {
		agents: [
			agentReadiness("claude-code", "Claude Code"),
			agentReadiness("codex", "Codex"),
			agentReadiness("cursor", "Cursor"),
		],
	});
	const onSubmit = vi.fn().mockResolvedValue(undefined);
	render(
		<QueryClientProvider client={queryClient}>
			<TooltipProvider>
				<CreateWorkProjectSheet open isCreating={false} onOpenChange={() => undefined} onSubmit={onSubmit} />
			</TooltipProvider>
		</QueryClientProvider>,
	);
	return onSubmit;
}

describe("CreateWorkProjectSheet", () => {
	it("keeps Create disabled until a name, an original, a manager and a worker are set", async () => {
		const user = userEvent.setup();
		window.ao!.app.chooseDirectory = vi.fn().mockResolvedValue("C:\\originals");
		const onSubmit = renderSheet();
		const create = screen.getByRole("button", { name: "Create" });
		expect(create).toBeDisabled();
		expect(screen.getByText("Choose which AI manages this project.")).toBeInTheDocument();
		expect(screen.getByText("Choose which AI does the work.")).toBeInTheDocument();

		await user.type(screen.getByLabelText("Name"), "Minutes");
		expect(create).toBeDisabled();

		await user.click(screen.getByRole("button", { name: "Add folder" }));
		expect(await screen.findByText("originals")).toBeInTheDocument();
		expect(create).toBeDisabled();

		await user.click(screen.getByLabelText("Manager"));
		expect(screen.getByRole("option", { name: /Cursor/ })).toHaveAttribute("aria-disabled", "true");
		expect(screen.getByRole("option", { name: /Cursor/ })).toHaveAttribute("title", "Work projects need an AI that Altair can watch.");
		await user.click(screen.getByRole("option", { name: /Claude Code/ }));
		expect(create).toBeDisabled();

		await user.click(screen.getByLabelText("Workers"));
		await user.click(screen.getByRole("option", { name: /Codex/ }));
		expect(create).toBeEnabled();

		await user.click(create);
		expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({
			name: "Minutes",
			sources: ["C:\\originals"],
			orchestratorAgent: "claude-code",
			workerAgent: "codex",
		}));
	});
});
