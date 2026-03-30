# Smailer: S3 Inbox Reader

Smailer is a fun, space-themed CLI tool built in Go using the Charm libraries (Bubble Tea, Lipgloss, etc.) for a terminal user interface (TUI). It allows you to browse and read emails stored in an AWS S3 bucket (e.g., from SES inbound emails under a prefix like "inbound/"). Emails are parsed, displayed in a paginated table, and rendered nicely in Markdown format. Features include deletion with confirmation, dynamic layouts, and a cosmic splash screen.


https://github.com/user-attachments/assets/ef558cb6-c49c-4076-b703-f9cfd6cf738d


## ✨ Features

- Pick a Bucket: If no `BUCKET` environment variable is set, it lists all S3 buckets (prioritizing those with "ses" in the name) for selection.
- Paginated Email List: Displays recent emails in a table with columns for From, Subject, Date, and a short key suffix for disambiguation. Loads more on scroll (10 at a time).
- Email Viewing: Hit Enter to load and view the email body, rendered as styled Markdown (HTML emails converted via html-to-markdown and Glamour).
- Save Raw Email: Press 's' to save the original S3 object as an `.eml` file in `~/Downloads/smailer`. Filenames are derived from the email date and subject.
- Filtering: Press `/` to filter the loaded emails by from, to, subject, or key.
- Attachment Saving: Press 'a' from the email view to save any attachments.
- Deletion: Press 'd' to delete from list or view, with a confirmation modal.

### Prerequisites

- AWS credentials configured (via `aws configure`, environment variables, or IAM roles)
- Permissions to manage an SES inbound s3 bucket

### Installation

#### Option 1: Install from Release (Recommended)

**One-line install script:**
```bash
curl -fsSL https://raw.githubusercontent.com/willfish/smailer/main/install | bash
```

**Manual download:**
```bash
# Download for your platform from releases
# Linux AMD64
curl -L -o smailer https://github.com/willfish/smailer/releases/latest/download/smailer-linux-amd64
chmod +x smailer
sudo mv smailer /usr/local/bin/

# Linux ARM64
curl -L -o smailer https://github.com/willfish/smailer/releases/latest/download/smailer-linux-arm64
chmod +x smailer
sudo mv smailer /usr/local/bin/

# macOS AMD64 (Intel)
curl -L -o smailer https://github.com/willfish/smailer/releases/latest/download/smailer-darwin-amd64
chmod +x smailer
sudo mv smailer /usr/local/bin/

# macOS ARM64 (Apple Silicon)
curl -L -o smailer https://github.com/willfish/smailer/releases/latest/download/smailer-darwin-arm64
chmod +x smailer
sudo mv smailer /usr/local/bin/
```

#### Option 2: Build from Source

```bash
# Clone the repository
git clone https://github.com/willfish/smailer
cd smailer

# Install dependencies
go mod tidy

# Build and install
go build -o smailer
sudo mv smailer /usr/local/bin/
```

#### Option 3: Go Install

```bash
go install github.com/willfish/smailer@latest
```

### Verify Installation

```bash
smailer
```

- If access denied and you're logged in to AWS you may need to export AWS profile first:

```bash
AWS_PROFILE=my-profile smailer
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Supported Platforms

| OS      | Architecture | Binary Name                  |
|---------|-------------|------------------------------|
| Linux   | AMD64       | `smailer-linux-amd64`        |
| Linux   | ARM64       | `smailer-linux-arm64`        |
| macOS   | AMD64       | `smailer-darwin-amd64`       |
| macOS   | ARM64       | `smailer-darwin-arm64`       |

## Release Builds

The project uses automated releases that build for multiple platforms:

- **Linux**: AMD64, ARM64
- **macOS**: AMD64 (Intel), ARM64 (Apple Silicon)

Binaries are automatically built and uploaded to GitHub Releases using the naming convention:
`smailer-{os}-{arch}`
