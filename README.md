# Webhook Relay

Webhook relay/fan-out service in Go. Receives incoming webhooks via HTTP, stores them in Postgres, publishes to a Redis Stream, and a worker fans out deliveries to registered actions with retry logic, idempotency, and HMAC signing.

## CI/CD

The GitHub Actions workflow (`.github/workflows/deploy.yml`) builds Docker images, pushes them to OCI Container Registry, and deploys the Helm chart to an OKE cluster. It runs on every push to `main`.

### Required GitHub Configuration

#### Repository Variables

Set these under **Settings > Secrets and variables > Actions > Variables**:

| Variable | Description | Example |
|---|---|---|
| `OCI_REGION` | OCI region identifier | `us-ashburn-1` |
| `OCIR_NAMESPACE` | Tenancy object storage namespace | `axwyz1234abcd` |
| `OCI_USERNAME` | OCIR login username | `oracleidentitycloudservice/you@email.com` |

#### Repository Secrets

Set these under **Settings > Secrets and variables > Actions > Secrets**:

| Secret | Description |
|---|---|
| `OCI_TENANCY_OCID` | OCID of the OCI tenancy |
| `OCI_USER_OCID` | OCID of the OCI user |
| `OCI_FINGERPRINT` | Fingerprint of the OCI API signing key |
| `OCI_PRIVATE_KEY` | Full PEM-encoded API signing private key |
| `OCI_AUTH_TOKEN` | Auth token for OCIR login (generate under User Settings > Auth Tokens) |
| `OKE_CLUSTER_OCID` | OCID of the target OKE cluster |
