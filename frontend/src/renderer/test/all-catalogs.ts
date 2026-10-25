// Every translation catalog, registered on the shared i18n instance. The app
// loads non-English catalogs on demand; tests that read another locale
// synchronously (getFixedT, t after changeLanguage) import this file first.
// It is not in the global setup because loading all catalogs costs ~0.3s per
// test file. instance.test.ts covers the lazy path.
import { appI18n } from "../i18n";
import { appCatalogs } from "../i18n/all-messages";

for (const [locale, catalog] of Object.entries(appCatalogs)) {
	appI18n.addResourceBundle(locale, "translation", catalog, true, true);
}
