# Security Policy

The DataTug project takes the security and integrity of our codebase, user data, and distributed components seriously. This policy outlines our vulnerability reporting process and supported version commitments.

---

## Supported Versions

We actively provide security patches for the following versions of DataTug Core:

| Version | Supported |
| :--- | :--- |
| `main` (active development) | :white_check_mark: Yes |
| Latest minor release (v0.x) | :white_check_mark: Yes |
| Older releases (< v0.x-1) | :x: No (please upgrade) |

---

## Reporting a Vulnerability

**Please do not report security vulnerabilities via public GitHub issues, discussions, or pull requests.**

If you discover or suspect a security vulnerability in DataTug Core:

1. **Email us directly**: Send a detailed report to **`security@datatug.app`**.
2. **Encrypted communication**: If desired, request our security team's PGP key prior to sending sensitive payloads.
3. **What to include**:
   - A clear description of the vulnerability, including attack vector and impact.
   - Exact steps or minimal reproducible code/fixture to reproduce the issue.
   - Affected packages or components (e.g., `pkg/meaninggraph`, `pkg/storage`, `pkg/recordsetcompare`).
   - Any suggested mitigations or patches if you have developed them.

---

## Our Vulnerability Response Process

1. **Acknowledgment**: Our security team will acknowledge receipt of your vulnerability report within **48 hours**.
2. **Triage & Assessment**: We will assess the severity and impact, verifying the report in an isolated environment within **5 business days**.
3. **Remediation**: We will prepare a fix, review it internally, and cut a security release.
4. **Coordinated Disclosure**: We aim to release a patch within **30 days** of initial notification. We will coordinate with you regarding the public disclosure date and credit your contribution in release notes (unless you request to remain anonymous).

---

## Security Best Practices for Deployments

- **Path Boundaries**: DataTug validates fixture and data paths against symlink traversal attacks. Avoid disabling path safety verifications in custom drivers.
- **Database Credentials**: Ensure sensitive database connection strings are supplied via environment variables or secret managers, not checked into repository projects.
