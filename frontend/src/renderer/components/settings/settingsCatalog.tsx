import { Activity, BadgeCheck, Bot, CircleHelp, Globe2, Keyboard, RefreshCw, Settings2, type LucideIcon } from "lucide-react";
import { lazy, type ReactNode } from "react";
import type { TFunction } from "i18next";
import type { GlobalSettingsSection, HarnessDiagnosticsRequest } from "../../stores/ui-store";
import { BrowserDownloadsSection } from "./BrowserDownloadsSection";
import { BrowserProfilesSection } from "./BrowserProfilesSection";
import { CodexAccountsSection } from "./CodexAccountsSection";
import { GeneralSettingsSection } from "./GeneralSettingsSection";
import { HarnessSettingsSection } from "./HarnessSettingsSection";
import { KeyboardShortcutsContent } from "./KeyboardShortcutsContent";
import { ReportProblemContent } from "./ReportProblemContent";
import { SettingsSection } from "./SettingsSection";

/** The memory window's own blocks, loaded only when the page is opened: it
 * samples the machine every two seconds while it is on screen. */
const MemoryDiagnostics = lazy(async () => {
	const module = await import("../SessionMemoryPanel");
	return { default: module.MemoryDiagnostics };
});

const UpdatesSection = lazy(async () => {
	const module = await import("./UpdatesSection");
	return { default: module.UpdatesSection };
});

type CatalogContext = {
	cloudEnabled: boolean;
	developerMode: boolean;
	/** Developer mode with the Diagnostics toggle on — gates the memory and CPU page. */
	diagnostics: boolean;
	/** The org holds the coder capability: gates the bring-your-own-Coder page. */
	customCoder: boolean;
	/** The org is on self-serve billing: gates the Upgrade (plans and usage) page. */
	selfServeBilling: boolean;
	focusAgentId?: string;
	hostId?: string;
	harnessView?: "local" | "cloud";
	startLogin?: boolean;
	showDiagnostics?: HarnessDiagnosticsRequest;
	/** One page is shown, under its own title: a first heading that only
	 * rewords that title ("Keyboard shortcuts" on Shortcuts) is hidden. */
	singlePage?: boolean;
};

export type SettingsCatalogItem = {
	id: GlobalSettingsSection;
	icon: LucideIcon;
	label: (t: TFunction) => string;
	visible?: (context: CatalogContext) => boolean;
	/** Left out of the single-page "all" view; it has its own page in the nav.
	 * Diagnostics is a live monitor, not a preference: rendering it inside the
	 * whole-settings page would sample the machine whenever settings opens. */
	pageOnly?: boolean;
	render: (t: TFunction, titleHidden: boolean, context: CatalogContext) => ReactNode;
};

const globalSettingsCatalog: SettingsCatalogItem[] = [
	{
		id: "general",
		icon: Settings2,
		label: (t) => t("settings.general"),
		render: (_t, titleHidden) => <GeneralSettingsSection titleHidden={titleHidden} />,
	},
	{
		id: "harness",
		icon: Bot,
		label: (t) => t("settings.harness"),
		render: (_t, titleHidden, { focusAgentId, hostId, harnessView, startLogin, showDiagnostics }) => <HarnessSettingsSection focusAgentId={focusAgentId} {...(hostId ? { hostId } : {})} {...(harnessView ? { initialView: harnessView } : {})} {...(startLogin ? { startLogin } : {})} {...(showDiagnostics ? { showDiagnostics } : {})} titleHidden={titleHidden} />,
	},
	{
		id: "agents",
		icon: BadgeCheck,
		label: (t) => t("settings.agents"),
		// In developer mode, subscription accounts live on each CLI's Harness
		// Account tab instead.
		visible: ({ developerMode }) => !developerMode,
		render: (_t, titleHidden) => <CodexAccountsSection titleHidden={titleHidden} />,
	},
	{
		id: "browserProfiles",
		icon: Globe2,
		label: (t) => t("settings.browserProfiles"),
		render: (_t, titleHidden) => (
			<>
				<BrowserProfilesSection titleHidden={titleHidden} />
				<BrowserDownloadsSection />
			</>
		),
	},
	{
		id: "shortcuts",
		icon: Keyboard,
		label: (t) => t("settings.shortcuts"),
		render: (_t, titleHidden, { singlePage }) => <KeyboardShortcutsContent active titleHidden={titleHidden || Boolean(singlePage)} />,
	},
	{
		id: "diagnostics",
		icon: Activity,
		label: (t) => t("settings.diagnostics"),
		pageOnly: true,
		visible: ({ diagnostics }) => diagnostics,
		render: (t, titleHidden) => (
			<SettingsSection titleHidden={titleHidden} title={t("settings.diagnostics")}>
				<MemoryDiagnostics />
			</SettingsSection>
		),
	},
	{
		id: "updates",
		icon: RefreshCw,
		label: (t) => t("settings.updates"),
		render: (_t, titleHidden) => <UpdatesSection titleHidden={titleHidden} />,
	},
	{
		id: "help",
		icon: CircleHelp,
		label: (t) => t("settings.help"),
		render: (t, titleHidden) => (
			<SettingsSection titleHidden={titleHidden} title={t("settings.reportProblem")} grouped>
				<div className="settings-content-row">
					<ReportProblemContent active />
				</div>
			</SettingsSection>
		),
	},
];

export function visibleGlobalSettings(context: CatalogContext): SettingsCatalogItem[] {
	return globalSettingsCatalog.filter((item) => item.visible?.(context) ?? true);
}

export function globalSettingsItem(section: GlobalSettingsSection, context: CatalogContext): SettingsCatalogItem {
	return visibleGlobalSettings(context).find((item) => item.id === section) ?? globalSettingsCatalog[0];
}

export function globalSettingsItemsFor(section: GlobalSettingsSection | "all", context: CatalogContext): SettingsCatalogItem[] {
	return section === "all"
		? visibleGlobalSettings(context).filter((item) => !item.pageOnly)
		: [globalSettingsItem(section, context)];
}
