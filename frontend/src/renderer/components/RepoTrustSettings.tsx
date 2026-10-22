import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { ProjectSettingsSection } from "@aoagents/product-ui";
import { apiClient, apiErrorMessage } from "../lib/api-client";
import { clientForHost } from "../lib/host-clients";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";

export function RepoTrustSettings({ projectId, hostId }: { projectId: string; hostId?: string }) {
 const client = hostId ? clientForHost(hostId) : apiClient;
 const [branch, setBranch] = useState("");
 const params = { path: { id: projectId } };
 const query = useQuery({
  queryKey: ["repo-trust", hostId, projectId, branch],
  queryFn: async () => {
   const { data, error } = await client.GET("/api/v1/projects/{id}/repo-trust", { params: { ...params, query: { branch } } });
   if (error) throw new Error(apiErrorMessage(error));
   return data;
  },
 });
 const mutation = useMutation({
  mutationFn: async (revoke: boolean) => {
   if (!query.data) return;
   const result = revoke
    ? await client.DELETE("/api/v1/projects/{id}/repo-trust", { params })
    : await client.POST("/api/v1/projects/{id}/repo-trust", { params, body: { fingerprint: query.data.fingerprint, branch } });
   if (result.error) throw new Error(apiErrorMessage(result.error));
  },
  onSettled: () => { void query.refetch(); },
 });
 return <ProjectSettingsSection title="Repository configuration trust" grouped>
  <div className="settings-content-row flex flex-col gap-3">
   <p className="text-sm text-settings-muted">Review commands and servers before allowing unattended agents to load repository configuration. Changes require approval again.</p>
   <Label htmlFor="repo-trust-branch">Worker branch (optional)</Label>
   <Input id="repo-trust-branch" value={branch} onChange={(event) => setBranch(event.target.value)} placeholder="Base checkout only" />
   {query.isPending ? <p className="text-sm text-settings-muted">Scanning repository configuration…</p> : null}
   {query.data?.files.map((file) => <div key={file.path}>
    <p className="font-mono text-sm">{file.path}</p>
    <ul className="text-sm text-settings-muted">{file.summary.map((line, index) => <li key={index} className="break-all whitespace-pre-wrap">{line}</li>)}</ul>
   </div>)}
   {query.data && query.data.files.length === 0 ? <p className="text-sm text-settings-muted">No repository agent configuration detected.</p> : null}
   {query.data?.fingerprint ? <>
    <p className="break-all font-mono text-xs text-settings-muted">{query.data.fingerprint}</p>
    <p className="text-sm">{query.data.approved ? "Approved" : "Approval required"}</p>
    <div className="flex gap-2">
     <Button type="button" disabled={mutation.isPending || query.isFetching || query.data.approved} onClick={() => mutation.mutate(false)}>Approve repository configuration</Button>
    </div>
   </> : null}
   {query.data ? <div><Button type="button" variant="ghost" disabled={mutation.isPending || query.isFetching} onClick={() => mutation.mutate(true)}>Revoke trust</Button></div> : null}
   {query.error || mutation.error ? <p role="alert" className="text-sm text-error">{(mutation.error ?? query.error)?.message}</p> : null}
  </div>
 </ProjectSettingsSection>;
}
