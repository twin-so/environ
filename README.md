# Environ: secrets out the repo, yet versioned
Environ is a minimalist tool designed for developers who need an efficient and maintainable way to manage secrets and configuration files within Git workflows—without compromising security or introducing complex overhead.

## Why Environ?

### The problem
- You want to keep .env files or sensitive configuration in your repo.
- You know storing secrets in plaintext is risky.
- You’re tired of heavyweight solutions and complex setups.

### The solution
- Indirection-based storage: Secrets live safely encrypted in your cloud bucket (e.g., AWS S3).
- Git-native integration: Store a simple version reference in your repo instead of plaintext secrets.
- Automated updates: Changes are seamlessly synchronized whenever you update the reference.

## How does it work?
1. Store secrets encrypted in your cloud bucket.
2. Define references in a lightweight file within your repo.
3. Environ CLI hooks into Git, automatically fetching the right secrets whenever references change.

## Simple by design
- Minimal overhead. Less reinventing the wheel, more smart integration.
- Seamless developer experience. Less complexity, more productivity.
- Secure from the start. Less risk, more peace of mind.

## Contribute & Feedback
Want to improve Environ? Open an issue, send a PR, or just reach out—we love hearing from fellow devs.

----
Built by the team at [Twin](https://twin.so). We build reliable and scalable autonomous agents for fintech.
We’re hiring! Check out our [careers page](https://twin.so/careers).
