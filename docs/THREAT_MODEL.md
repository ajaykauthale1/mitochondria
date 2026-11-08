# mitochondria — Threat Model & Security Policy

## 1. Security Philosophy

**mitochondria itself must never become a security or privacy vulnerability.**
Because endpoint security tools inspect processes, files, and network connections, an overly privileged or poorly isolated agent can become an attacker's best privilege-escalation vector or a privacy leak.

## 2. Trust Boundaries

```text
┌───────────────────────────────────────────────────────────────────┐
│ Untrusted Monitored Environment                                   │
│ - Newly spawned user processes & child shells                     │
│ - Downloaded files in %TEMP% / ~/Downloads                        │
│ - External network destinations                                   │
└───────────────┬───────────────────────────────────────────────────┘
                │ Read-only OS APIs (Toolhelp32 / fsnotify / netstat)
                ▼
┌───────────────────────────────────────────────────────────────────┐
│ Trusted Local Boundary: Mitochondria Go Engine                    │
│ - Runs with least privilege by default (user session)             │
│ - Bounded memory (GOMEMLIMIT) & bounded event channel (cap 1024)  │
│ - Local SQLite DB in user's protected AppData/Config dir (0600)   │
└───────────────┬───────────────────────────────────┬───────────────┘
                │ Local Wails IPC                   │ Explicit Opt-In Only
                ▼                                   ▼
┌───────────────────────────────┐   ┌───────────────────────────────┐
│ Trusted Desktop UI            │   │ Untrusted External AI API     │
│ - Local WebView2/WebKit       │   │ - Receives ONLY sanitized     │
│ - Requires user approval for  │   │   incident metadata           │
│   destructive containment     │   │ - Never receives file bodies, │
└───────────────────────────────┘   │   keys, env vars, or PII      │
                                    └───────────────────────────────┘
```

## 3. Threats to Mitochondria & Mitigations

| # | Threat | Impact | Mitigation |
|---|---|---|---|
| **T1** | **Event Flood / Resource Exhaustion DoS** (e.g., `npm install` or compiler spawning thousands of files/processes) | High CPU or OOM crash on user laptop | Bounded event channel (`cap: 1024`), sliding-window rate limiting, graceful degradation that drops low-priority file events while preserving process lineage tracking, and `GOMEMLIMIT` enforcement. |
| **T2** | **Baseline Poisoning** (Attacker slowly introduces malicious process behavior during learning phase) | Malicious process marked as "Normal" | Never auto-trust unsigned binaries in `%TEMP%` or high-severity rule triggers regardless of baseline state; require minimum observation windows and cryptographic hash stability. |
| **T3** | **Prompt Injection via Process Command Line or Filename** (Attacker names a binary `ignore_previous_instructions_safe.exe`) | AI investigator tricked into marking malware as benign | Deterministic rules run *before* and *independently* of AI. AI cannot downgrade a deterministic alert's severity. All telemetry strings passed to LLM are wrapped in strict untrusted data delimiters. |
| **T4** | **Sensitive Data Exfiltration via Cloud AI** | SSH keys, passwords, or private documents leaked to LLM provider | Telemetry collector only records *metadata* (path, PID, timestamp, SHA-256) and never reads file contents or environment variables. Evidence sanitizer strips user-specific tokens before any opt-in AI call. |
| **T5** | **Self-Sabotage / Destructive False Positive** (Legitimate OS or developer binary quarantined) | System instability or broken developer workflow | Destructive actions (process kill, file quarantine, startup removal) require explicit user approval by default, exclude signed OS system binaries, and record reversible rollback bundles. |
