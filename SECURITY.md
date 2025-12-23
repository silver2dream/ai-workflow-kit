# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue, please report it responsibly.

### How to Report

1. **DO NOT** create a public GitHub issue for security vulnerabilities
2. Email security concerns to: [security@example.com] (replace with actual email)
3. Or use GitHub's private vulnerability reporting feature:
   - Go to the repository's "Security" tab
   - Click "Report a vulnerability"

### What to Include

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

### Response SLA

| Severity | Initial Response | Resolution Target |
|----------|-----------------|-------------------|
| Critical | 24 hours        | 7 days            |
| High     | 48 hours        | 14 days           |
| Medium   | 7 days          | 30 days           |
| Low      | 14 days         | 90 days           |

### What to Expect

1. **Acknowledgment**: We will acknowledge receipt of your report within the SLA timeframe
2. **Investigation**: We will investigate and validate the vulnerability
3. **Fix Development**: We will develop and test a fix
4. **Disclosure**: We will coordinate disclosure timing with you
5. **Credit**: We will credit you in the security advisory (unless you prefer anonymity)

## Security Best Practices

When using AWK (AI Workflow Kit):

1. **Never commit secrets** - Use environment variables or secret management
2. **Review AI-generated code** - Always review before merging
3. **Use branch protection** - Require reviews and CI checks
4. **Keep dependencies updated** - Enable Dependabot alerts
5. **Monitor audit logs** - Review `.ai/state/` logs regularly

## Security Features

AWK includes several security features:

- **Path isolation**: Worker cannot access Principal session data
- **Audit logging**: All operations are logged with session IDs
- **GitHub integration**: Uses GitHub's security features (branch protection, required reviews)
- **Secret scanning**: Checks for sensitive information in code changes

### GitHub Security Settings (Recommended)

Enable these features in your repository settings:

1. **Secret Scanning**: Settings → Security → Secret scanning → Enable
2. **Push Protection**: Settings → Security → Push protection → Enable
3. **Dependabot Alerts**: Settings → Security → Dependabot alerts → Enable
4. **CodeQL Analysis**: Automatically enabled via `.github/workflows/codeql.yml`

## Acknowledgments

We thank the following individuals for responsibly disclosing security issues:

- (None yet - be the first!)
