// Shared test setup for every environment. Keep it light: it runs before each
// test file, so an import here is paid ~500 times per suite run.

export {};

// Pin the default formatting locale. Tests assert en-US output ("18,055",
// "09:30"), but Node takes its default locale from the host, and on Windows ICU
// reads it from the OS, so LANG cannot override it (an ar-EG host prints
// "١٨٬٠٥٥"). Calls that pass an explicit locale are unchanged.
const TEST_LOCALE = "en-US";
for (const name of ["Collator", "DateTimeFormat", "ListFormat", "NumberFormat", "PluralRules", "RelativeTimeFormat"] as const) {
	const original = Intl[name] as unknown as (...args: unknown[]) => unknown;
	(Intl as Record<string, unknown>)[name] = new Proxy(original, {
		construct: (target, [locales, ...rest]) =>
			Reflect.construct(target, [locales ?? TEST_LOCALE, ...rest]) as object,
		apply: (target, thisArg, [locales, ...rest]) => Reflect.apply(target, thisArg, [locales ?? TEST_LOCALE, ...rest]),
	});
}
const pinLocale = <T>(proto: T, method: keyof T & string) => {
	const original = proto[method] as unknown as (...args: unknown[]) => unknown;
	Object.defineProperty(proto, method, {
		configurable: true,
		writable: true,
		value(this: unknown, locales?: unknown, ...rest: unknown[]) {
			return original.call(this, locales ?? TEST_LOCALE, ...rest);
		},
	});
};
pinLocale(Number.prototype, "toLocaleString");
pinLocale(BigInt.prototype, "toLocaleString");
pinLocale(Date.prototype, "toLocaleString");
pinLocale(Date.prototype, "toLocaleDateString");
pinLocale(Date.prototype, "toLocaleTimeString");
const localeCompare = String.prototype.localeCompare;
Object.defineProperty(String.prototype, "localeCompare", {
	configurable: true,
	writable: true,
	value(this: string, that: string, locales?: Intl.LocalesArgument, options?: Intl.CollatorOptions) {
		return localeCompare.call(this, that, locales ?? TEST_LOCALE, options);
	},
});

// DOM stubs and jest-dom load only where a DOM exists; node-environment tests
// would pay for the imports without using them.
if (typeof window !== "undefined") {
	await import("./setup-dom");
}
