import { useIsMutating, useMutation, useMutationState, useQueryClient } from "@tanstack/react-query";
import { awakeCapability, awakeQueryKey, type AwakeState, useAwake } from "./useAwake";

type Change = { on: boolean; lidClosed?: boolean };
const mutationKey = ["stay-awake-toggle"] as const;

/** Stay Awake is the desktop awake service alone. */
export function useStayAwake(enabled = true) {
	const cache = useQueryClient();
	const awake = useAwake(enabled);
	const sessions = awake.state?.sessions?.filter((s) => !s.endedAt) ?? [];
	const rules = awake.state?.paused ? [] : awake.state?.rules?.filter((r) => r.enabled && !r.suppressed) ?? [];
	const desktopOn = sessions.length > 0 || rules.length > 0 || !!awake.state?.active;
	const desktopLid = sessions.some((s) => s.lidClosed) || rules.some((r) => r.lidClosed) || !!awake.state?.lidActive;
	const pending = useMutationState<Change>({
		filters: { mutationKey, status: "pending" },
		select: (m) => m.state.variables as Change,
	});
	const intent = pending.at(-1);
	const busy = useIsMutating({ mutationKey }) > 0 || awake.busy;
	// Requested ownership must stay clearable even when a rule is waiting or a
	// safety stop has released the host assertion. Actual wake is separate.
	const active = !!awake.state?.active;
	const on = intent?.on ?? (desktopOn || active);
	// Status also includes recovery/lid errors unrelated to an inactive request.
	const pauseReason = awake.state?.paused ? awake.state.reason : undefined;
	const reason = pauseReason === "low_battery" || pauseReason === "critical_heat" || pauseReason === "battery_unknown"
		? pauseReason : undefined;
	const mutation = useMutation({
		mutationKey,
		mutationFn: async (change: Change) => {
			const start = async () => {
				let latest = await awake.actAsync({ kind: "start", request: { scope: "ao", sessionId: "", mode: "indefinite", lidClosed: false } });
				// A successful POST writes a lease; the native helper acknowledges it
				// on its next two-second scan. Keep the shared intent pending until
				// that acknowledgement instead of flashing an inactive warning.
				const deadline = Date.now() + 5_000;
				while (!latest.active && !latest.paused && (!latest.reason || latest.reason === "starting" || latest.reason === "helper_unavailable") && Date.now() < deadline) {
					await new Promise((resolve) => setTimeout(resolve, 500));
					let timeout: number | undefined;
					try {
						const confirmed = await Promise.race([
							cache.fetchQuery<AwakeState>({ queryKey: awakeQueryKey, staleTime: 0 }),
							new Promise<undefined>((resolve) => { timeout = window.setTimeout(resolve, Math.max(0, deadline - Date.now())); }),
						]);
						if (!confirmed) break;
						latest = confirmed;
					} catch {
						// An unavailable status endpoint must not leave the controls busy.
						break;
					} finally {
						window.clearTimeout(timeout);
					}
				}
			};
			const ensureDesktop = async () => {
				if (awake.state?.paused) await awake.actAsync({ kind: "resume" });
				if (sessions.length > 0) return;
				try {
					await start();
				} catch (e) {
					// A safety stop can happen after the last poll. Re-read the daemon's
					// structured pause state rather than interpreting its error wording.
					await cache.cancelQueries({ queryKey: awakeQueryKey });
					const latest = await cache.fetchQuery<AwakeState>({ queryKey: awakeQueryKey, staleTime: 0 }).catch(() => undefined);
					if (!latest?.paused) throw e;
					await awake.actAsync({ kind: "resume" });
					await start();
				}
			};
			if (!change.on) {
				await awake.actAsync({ kind: "stop" });
			} else if (change.lidClosed === false) {
				if (desktopLid) {
					// Retire legacy desktop lid owners and replace them with idle-only wake.
					await awake.actAsync({ kind: "stop" });
					await start();
				}
			} else {
				await ensureDesktop();
			}
		},
	});
	const change = (next: Change) => {
		// The cache guard also covers two calls before React renders the disabled UI.
		if (cache.isMutating({ mutationKey }) > 0 || awake.busy) return;
		mutation.mutate(next);
	};
	return {
		on,
		active,
		attention: on && !active && !busy,
		reason: desktopOn ? reason : undefined,
		lidOn: intent ? intent.on && (intent.lidClosed ?? false) : desktopLid,
		// The second argument is retained so existing callers keep compiling.
		// Mobile wake no longer has a backend in this build.
		set: (on: boolean, _rememberMobile = false) => change({ on }),
		setLid: (lidClosed: boolean) => change({ on: true, lidClosed }),
		busy,
		error: mutation.error instanceof Error ? mutation.error.message : null,
		supported: awakeCapability(awake.state, "idle")?.available ?? false,
		starting: intent?.on === true && !active,
	};
}
