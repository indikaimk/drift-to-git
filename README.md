# drift-to-git 🔄

**drift-to-git** is a reverse GitOps reconciler for Kubernetes and Argo CD. It detects live cluster modifications (such as emergency `kubectl edit` hotfixes or manual patches), filters out runtime noise, generates clean **Kustomize strategic merge patches**, and opens a **GitHub Pull Request** with your Personal Access Token (PAT).

---

## 💡 Why `drift-to-git`?

In standard GitOps with Argo CD:
1. An engineer performs an emergency hotfix during an incident (`kubectl edit deployment payment-api`).
2. If `selfHeal: true` is on, Argo CD reverts the hotfix immediately, causing downtime.
3. If `selfHeal: false` is on, the cluster drifts, but someone must manually review and create a PR to reconcile Git with the cluster.
4. **`drift-to-git` bridges this gap**: it captures live cluster changes and creates a ready-to-merge GitHub Pull Request, bringing Git and the cluster back into sync without wiping out hotfixes.

---

## 🛠️ How It Works

```
┌─────────────────┐       ┌─────────────────┐       ┌────────────────────────┐
│ Kubernetes Live │ ───>  │     Argo CD     │ ───>  │      drift-to-git      │
│  (kubectl edit) │       │  (OutOfSync)    │       │ (filters noise, builds │
└─────────────────┘       └─────────────────┘       │   Kustomize patch)     │
                                                            │
                                                            ▼
                                                    ┌────────────────────────┐
                                                    │   GitHub Pull Request  │
                                                    │ (opens branch + PR)    │
                                                    └────────────────────────┘
```

1. **Argo CD Integration**: Identifies applications in `OutOfSync` state and extracts desired (`targetState`) vs live (`liveState`) manifests via the Argo CD API.
2. **Noise & Status Filtering**: Strips runtime noise (`status`, `managedFields`, `resourceVersion`, `uid`, `generation`, `creationTimestamp`, and system annotations).
3. **Kustomize Patch Generation**: Computes user-space deltas and creates a minimal strategic merge patch (e.g. `patches/drift-deployment-my-app.yaml`).
4. **Kustomization AST Updates**: Appends the patch path under `patches:` in `kustomization.yaml` while preserving comments and indentation.
5. **Atomic GitHub PR**: Uses GitHub REST API (with PAT) to create a branch, commit files, and open a Pull Request.
6. **Idempotency Protection**: Checks if an open PR already exists for the application before creating a new one to prevent PR spam.

---

## 🚀 Quick Start

### 1. Build the Binary

```bash
make build
# Binary created at bin/drift-to-git
```

### 2. Run a Dry Run Scan (Local / CI)

```bash
export ARGOCD_SERVER="https://argocd.example.com"
export ARGOCD_AUTH_TOKEN="your-argocd-token"

# Inspect drift without modifying GitHub
./bin/drift-to-git scan --dry-run
```

### 3. Run Live PR Creation

```bash
export ARGOCD_SERVER="https://argocd.example.com"
export ARGOCD_AUTH_TOKEN="your-argocd-token"
export GITHUB_TOKEN="ghp_your_github_pat"

./bin/drift-to-git scan
```

---

## ⚙️ Configuration Reference

| Environment Variable | CLI Flag | Default | Description |
| :--- | :--- | :--- | :--- |
| `ARGOCD_SERVER` | `--argocd-server` | `http://localhost:8080` | URL of the Argo CD API server |
| `ARGOCD_AUTH_TOKEN` | `--argocd-token` | `""` | Bearer token for Argo CD API |
| `ARGOCD_INSECURE` | | `true` | Skip TLS certificate verification |
| `ARGOCD_APP_FILTER` | `--app` | `""` | Comma-separated list of application names to monitor |
| `GITHUB_TOKEN` | `--github-token` | `""` | GitHub Personal Access Token (PAT) with `repo` scope |
| `GITHUB_DEFAULT_BRANCH` | | `main` | Fallback branch if not in Argo CD `targetRevision` |
| `GITHUB_BRANCH_PREFIX` | | `drift-to-git` | Prefix for generated branches (`drift-to-git/<app>-<timestamp>`) |
| `PATCH_DIR` | | `patches` | Subfolder within application directory to store patch files |
| `DRY_RUN` | `--dry-run` | `false` | Log patches without opening PRs |
| `POLL_INTERVAL` | `--poll-interval` | `60s` | Polling interval when running in daemon / operator mode |

---

## ☸️ In-Cluster Operator Deployment

To deploy `drift-to-git` directly inside your Kubernetes cluster (e.g., in the `argocd` namespace):

```bash
# 1. Configure secrets
cp deploy/secret.yaml.example deploy/secret.yaml
# Edit deploy/secret.yaml with your GITHUB_TOKEN

kubectl apply -f deploy/secret.yaml
kubectl apply -f deploy/rbac.yaml
kubectl apply -f deploy/deployment.yaml
```

### Application Annotations

You can selectively opt-out specific Argo CD applications from drift reconciliation:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: non-critical-app
  annotations:
    drift-to-git.io/enabled: "false" # drift-to-git will ignore this app
```

---

## 🧪 Running Tests

```bash
make test
```
