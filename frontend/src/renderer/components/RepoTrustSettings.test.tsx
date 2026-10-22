import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { RepoTrustSettings } from "./RepoTrustSettings";

const { get, post, del } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), del: vi.fn() }));
vi.mock("../lib/api-client", () => ({ apiClient: { GET: get, POST: post, DELETE: del }, apiErrorMessage: () => "Configuration changed; review again" }));
vi.mock("../lib/host-clients", () => ({ clientForHost: () => ({ GET: get, POST: post, DELETE: del }) }));
const inventory = { fingerprint: "abc", approved: false, files: [{ path: ".mcp.json", summary: ["mcpServers.example.command: touch marker"] }] };
beforeEach(() => {
 get.mockReset().mockResolvedValue({ data: inventory });
 post.mockReset().mockResolvedValue({ data: { ...inventory, approved: true } });
 del.mockReset().mockResolvedValue({ data: inventory });
});
function show() {
 const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
 render(<QueryClientProvider client={client}><RepoTrustSettings projectId="p" /></QueryClientProvider>);
}
it("shows files and commands before sending the exact reviewed fingerprint", async () => {
 show();
 expect(await screen.findByText(".mcp.json")).toBeVisible();
 expect(screen.getByText("mcpServers.example.command: touch marker")).toBeVisible();
 expect(post).not.toHaveBeenCalled();
 await userEvent.click(screen.getByRole("button", { name: "Approve repository configuration" }));
 await waitFor(() => expect(post).toHaveBeenCalledWith("/api/v1/projects/{id}/repo-trust", { params: { path: { id: "p" } }, body: { fingerprint: "abc", branch: "" } }));
});
it("revokes trust and surfaces stale-approval failures", async () => {
 show(); await screen.findByText(".mcp.json");
 await userEvent.click(screen.getByRole("button", { name: "Revoke trust" }));
 await waitFor(() => expect(del).toHaveBeenCalledOnce());
 post.mockResolvedValueOnce({ error: { error: { code: "REPO_TRUST_STALE" } } });
 await userEvent.click(screen.getByRole("button", { name: "Approve repository configuration" }));
 expect(await screen.findByRole("alert")).toHaveTextContent("Configuration changed; review again");
});
it("keeps config-free projects free of an approval action", async () => {
 get.mockResolvedValue({ data: { fingerprint: "", approved: false, files: [] } });
 show(); expect(await screen.findByText("No repository agent configuration detected.")).toBeVisible();
 expect(screen.queryByRole("button", { name: "Approve repository configuration" })).not.toBeInTheDocument();
});
