# Security Policy

## Supported Versions

The following versions of AWK (AI Workflow Kit) are currently supported with security updates:

| Version | Supported |
| ------- | --------- |
| 1.x.x   | ✅ Yes    |
| < 1.0   | ❌ No     |

Only supported versions will receive security patches. Users are strongly encouraged to upgrade to a supported release.

---

## Reporting a Vulnerability

We take security vulnerabilities seriously and appreciate responsible disclosure.

### How to Report

If you believe you have found a security vulnerability, please follow **one** of the options below:

1. **DO NOT** open a public GitHub issue.

2. Use GitHub's **Private Vulnerability Reporting** feature:
   - Go to the repository's **Security** tab
   - Click **Report a vulnerability**

3. (Optional) Email security concerns to:  
   **security@YOURDOMAIN.COM**  
   *(Replace with a real address if available. If not, GitHub reporting is preferred.)*

### What to Include

Please include as much information as possible:

- A clear description of the vulnerability
- Steps to reproduce
- Affected versions
- Potential impact and severity
- Suggested mitigation or fix (if known)

---

## Response SLA

We aim to respond and remediate according to the following targets:

| Severity  | Initial Response | Resolution Target |
|-----------|------------------|-------------------|
| Critical  | Within 24 hours  | 7 days            |
| High      | Within 48 hours  | 14 days           |
| Medium    | Within 7 days    | 30 days           |
| Low       | Within 14 days   | 90 days           |

### What to Expect

1. **Acknowledgment** – Confirmation that we received your report
2. **Investigation** – Validation and impact assessment
3. **Fix Development** – Patch development and testing
4. **Coordinated Disclosure** – Disclosure timing agreed with the reporter
5. **Credit** – Public acknowledgment unless anonymity is requested

---

## Security Assurance & Verification

This project follows commonly accepted open source security best practices.  
Security-related signals are **publicly verifiable** in this repository, including:

- Automated dependency vulnerability scanning (Dependabot)
- Static application security testing (GitHub CodeQL)
- OpenSSF Scorecard monitoring
- GitHub branch protection and required pull request reviews
- Secret scanning for known credential patterns

Users and reviewers are encouraged to inspect the repository's **Security** tab for up-to-date results.

---

## Threat Model & Scope

AWK is designed to mitigate risks related to:

- Accidental exposure of secrets in AI-generated code
- Unauthorized access between isolated AI worker sessions
- Unreviewed automated code changes
- Incomplete auditability of AI-driven workflows

AWK **does not** protect against:

- Malicious actions by trusted contributors
- Compromised GitHub accounts or access tokens
- Vulnerabilities in third-party dependencies or AI model providers
- Misconfiguration of repository permissions or branch protection

Security is a shared responsibility between the tool and its users.

---

## Supply Chain Security

- Source code and installation scripts are hosted in this repository
- Releases are distributed via GitHub Releases
- Installation scripts are versioned and subject to code review
- Users are strongly encouraged to **pin exact versions** instead of using `latest`
- No binaries or scripts are fetched from untrusted third-party sources at install time

---

## Security Best Practices for Users

When using AWK (AI Workflow Kit), users are responsible for:

- Reviewing all AI-generated code before merging
- Managing GitHub tokens using least-privilege scopes
- Enabling branch protection and required reviews
- Keeping dependencies up to date
- Monitoring audit logs and workflow outputs
- Ensuring compliance with their organization's security policies

---

## Built-in Security Features

AWK includes the following security-oriented design features:

- **Path isolation**: Worker sessions cannot access Principal session data
- **Audit logging**: All workflow operations are logged with session identifiers
- **Explicit workflow states**: Issue/PR labels act as a visible state machine
- **GitHub-native controls**: Integrates with branch protection and review rules
- **Secret detection**: Detects common sensitive patterns in generated changes

---

## Acknowledgments

We thank the following individuals for responsibly disclosing security issues:

- *(No disclosures yet — be the first!)*
