# Proxy module token key

The proxy encrypts per-module GitHub tokens at rest with AES-256-GCM. Provide a
32-byte key, base64-encoded, via the `MODULE_TOKEN_KEY` environment variable.

Generate one:

    openssl rand -base64 32

Set it in the Railway service variables for the proxy. If unset, module
registration still works for public repos; submitting a GitHub token returns
"token storage not configured".
