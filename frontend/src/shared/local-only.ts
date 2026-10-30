/**
 * This build of Altair is local-only: the app and its daemon never
 * reach the network on their own initiative, and nothing listens beyond
 * loopback. There is no mobile app pairing, no remote control (remote hosts,
 * LAN listener, tunnels), no AO Cloud, no telemetry export, no automatic
 * update check or version-floor check, and no pricing or link-preview fetching.
 * Agent CLIs still talk to their own model providers, and git/GitHub/GitLab
 * work as before. The one update path is user-initiated: Check for updates in
 * Settings asks this build's own GitHub releases (main/manual-updater.ts).
 *
 * The subsystems' code is kept so they can come back later; each switch point
 * reads this flag. It is typed `boolean` (not the literal `true`) so the code
 * behind a disabled branch keeps type-checking.
 */
export const LOCAL_ONLY_BUILD: boolean = true;

/** User-facing reason a disabled feature is unavailable. */
export const LOCAL_ONLY_DISABLED_MESSAGE = "This feature is disabled in this local-only build of Altair.";
