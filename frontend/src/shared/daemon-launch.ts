export type DaemonLaunchSpec = {
	command: string;
	args: string[];
	cwd: string;
	shell: boolean;
	source: "configured" | "bundled" | "dev";
};

function joinPath(...segments: string[]): string {
	return segments.map((segment) => segment.replace(/[/\\]+$/, "")).join("/");
}

export function bundledDaemonBinaryName(platform: NodeJS.Platform): string {
	return platform === "win32" ? "ao.exe" : "ao";
}

/** Name linked by Developer Mode. The app still launches the daemon as `ao`. */
export function bundledCliBinaryName(platform: NodeJS.Platform): string {
	return platform === "win32" ? "altair.exe" : "altair";
}

const PRODUCT_INSTALL_DIR_NAMES = new Set([
	"agent orchestrator.app",
	"agent orchestrator",
	"orchestrator.inc",
	"altair.app",
	"altair",
]);

function pathParts(filePath: string): string[] {
	return filePath.replace(/\\/g, "/").split("/").filter((part) => part.length > 0);
}

/**
 * True when the path is ao/altair inside a daemon directory of this product,
 * including an install that still uses a previous folder name.
 */
export function isProductDaemonExecutable(filePath: string): boolean {
	const parts = pathParts(filePath);
	const base = (parts.at(-1) ?? "").toLowerCase();
	const name = base.endsWith(".exe") ? base.slice(0, -4) : base;
	if (name !== "ao" && name !== "altair") return false;
	if ((parts.at(-2) ?? "").toLowerCase() !== "daemon") return false;
	return parts.some((part) => PRODUCT_INSTALL_DIR_NAMES.has(part.toLowerCase()));
}

export function resolveDaemonLaunch(
	env: Record<string, string | undefined>,
	isPackaged: boolean,
	resourcesPath: string,
	appPath: string,
	homeDir: string,
	platform: NodeJS.Platform,
): DaemonLaunchSpec | null {
	const configuredCommand = env.AO_DAEMON_COMMAND?.trim();
	if (configuredCommand) {
		return {
			command: configuredCommand,
			args: [],
			cwd: appPath,
			shell: true,
			source: "configured",
		};
	}

	if (!isPackaged) {
		// Run the daemon predev already built rather than `go run`, so the daemon's
		// parent is Electron itself, as in the packaged app; the diagnostics view
		// counts the daemon's parent tree as the rest of AO.
		return {
			command: env.AO_DEV_DAEMON_BINARY?.trim() || joinPath(appPath, "daemon", bundledDaemonBinaryName(platform)),
			args: ["daemon"],
			cwd: appPath,
			shell: false,
			source: "dev",
		};
	}

	return {
		command: joinPath(resourcesPath, "daemon", bundledDaemonBinaryName(platform)),
		args: ["daemon"],
		cwd: joinPath(homeDir, ".ao"),
		shell: false,
		source: "bundled",
	};
}

export type BundledDaemonProbe = {
	executablePath?: string;
	appImagePath?: string;
};

/**
 * Identity check for a bundled daemon. Under AppImage the executable path is a
 * random /tmp/.mount_* path regenerated on every launch, so identity is the
 * stable outer .AppImage file path the daemon reports (AO_APPIMAGE, echoed as
 * appImagePath) — compared against this process's own APPIMAGE. Outside
 * AppImage the packaged executable path is stable and compared directly.
 *
 * Returns an error message, or null when the probed daemon belongs to this
 * install. A probe that cannot prove its identity (missing field) fails closed.
 */
export function bundledDaemonIdentityError(
	probe: BundledDaemonProbe,
	expectedCommand: string,
	appImagePath: string | undefined,
	samePath: (a: string, b: string) => boolean,
): string | null {
	if (appImagePath) {
		if (!probe.appImagePath) {
			return "An older Altair daemon is already running, but it does not report its install identity. Stop it and restart this app.";
		}
		if (!samePath(probe.appImagePath, appImagePath)) {
			return `Another Altair daemon is already running from ${probe.appImagePath}; expected ${appImagePath}. Stop the other daemon before using this app.`;
		}
		return null;
	}
	if (!probe.executablePath) {
		return "An older Altair daemon is already running, but it does not report its binary path. Stop it and restart this app.";
	}
	if (samePath(probe.executablePath, expectedCommand)) return null;
	// A previous install (Agent Orchestrator, Orchestrator.inc) is this product.
	// Attach to its daemon. A binary that is merely named ao is still foreign.
	if (isProductDaemonExecutable(probe.executablePath) && isProductDaemonExecutable(expectedCommand)) {
		return null;
	}
	return `Another Altair daemon is already running from ${probe.executablePath}; expected ${expectedCommand}. Stop the other daemon before using this app.`;
}
