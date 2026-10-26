# Worker secrets

Workers inherit a filtered environment and, when sandboxed, cannot read the
built-in protected-file list. Before each release, run `npm run canary`.
