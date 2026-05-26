# Examples

This directory contains example Kubernetes manifests for deploying and running the monitoring tool in various scenarios.

## Files

### `rbac.yaml`
RBAC permissions required to run the monitoring tool. Includes:
- ServiceAccount for the monitoring tool
- ClusterRole with read permissions for pods and metrics
- ClusterRoleBinding to grant permissions
- Optional Role for deployment operations

**Apply**:
```bash
kubectl apply -f examples/rbac.yaml
```

### `cronjob.yaml`
Example CronJob for scheduled monitoring runs. Includes:
- CronJob that runs daily at 2 AM
- ServiceAccount with required permissions
- PersistentVolumeClaim for storing reports

**Apply**:
```bash
kubectl apply -f examples/cronjob.yaml
```

## Usage Examples

### Running with ServiceAccount

If you've applied `rbac.yaml`, you can run the monitoring tool using the ServiceAccount:

```bash
# On the cluster (in a Pod)
kubectl run -it --rm monitor \
  --image=golang:1.22 \
  --serviceaccount=node-exporter-monitor \
  --namespace=node-exporter-zoneinfo \
  -- bash

# Inside the pod:
git clone <your-repo>
cd node-exporter-zoneinfo
make build
./bin/node-exporter-monitor --duration=30m
```

### Scheduled Monitoring

After applying `cronjob.yaml`:

```bash
# Check CronJob status
kubectl get cronjob -n node-exporter-zoneinfo

# Manually trigger a job
kubectl create job --from=cronjob/node-exporter-monitor manual-run-1 -n node-exporter-zoneinfo

# Check job status
kubectl get jobs -n node-exporter-zoneinfo

# View logs
kubectl logs -n node-exporter-zoneinfo job/manual-run-1

# Access reports (if using PVC)
kubectl exec -it <monitoring-pod> -n node-exporter-zoneinfo -- ls -la /workspace/reports
```

### One-off Kubernetes Job

Create a one-time Job to run monitoring:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: node-exporter-monitor-once
  namespace: node-exporter-zoneinfo
spec:
  template:
    spec:
      serviceAccountName: node-exporter-monitor
      restartPolicy: Never
      containers:
      - name: monitor
        image: golang:1.22
        command:
        - /bin/bash
        - -c
        - |
          cd /tmp
          git clone <your-repo-url>
          cd node-exporter-zoneinfo
          make build
          ./bin/node-exporter-monitor --duration=10m --format=json
          cat reports/*.json
EOF
```

### Testing RBAC Permissions

Verify the ServiceAccount has correct permissions:

```bash
# Test pod list permission
kubectl auth can-i list pods \
  --namespace=node-exporter-zoneinfo \
  --as=system:serviceaccount:node-exporter-zoneinfo:node-exporter-monitor

# Test metrics access
kubectl auth can-i get pods.metrics.k8s.io \
  --namespace=openshift-monitoring \
  --as=system:serviceaccount:node-exporter-zoneinfo:node-exporter-monitor
```

## Customization

### Adjust CronJob Schedule

Edit the schedule in `cronjob.yaml`:

```yaml
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  # schedule: "0 */6 * * *"  # Every 6 hours
  # schedule: "0 0 * * 0"     # Weekly on Sunday
  # schedule: "0 0 1 * *"     # Monthly on 1st
```

### Change Monitoring Duration

Modify the command in `cronjob.yaml`:

```yaml
command:
- /bin/bash
- -c
- |
  ./bin/node-exporter-monitor --duration=2h --interval=1m --format=html
```

### Store Reports in ConfigMap (for small reports)

Instead of PVC, use a ConfigMap:

```yaml
# After monitoring completes
kubectl create configmap monitoring-report-$(date +%Y%m%d) \
  --from-file=reports/ \
  -n node-exporter-zoneinfo
```

### Send Reports to S3/Object Storage

Add AWS CLI to the container and upload:

```yaml
command:
- /bin/bash
- -c
- |
  apt-get update && apt-get install -y awscli
  ./bin/node-exporter-monitor --duration=1h --format=json
  aws s3 cp reports/ s3://my-bucket/monitoring-reports/$(date +%Y%m%d)/ --recursive
env:
- name: AWS_ACCESS_KEY_ID
  valueFrom:
    secretKeyRef:
      name: aws-credentials
      key: access-key-id
- name: AWS_SECRET_ACCESS_KEY
  valueFrom:
    secretKeyRef:
      name: aws-credentials
      key: secret-access-key
```

## Troubleshooting

### Permission Errors

If you see permission errors:

1. Check ServiceAccount exists:
   ```bash
   kubectl get sa node-exporter-monitor -n node-exporter-zoneinfo
   ```

2. Verify ClusterRole and binding:
   ```bash
   kubectl get clusterrole node-exporter-monitor-reader
   kubectl get clusterrolebinding node-exporter-monitor-reader
   ```

3. Test permissions manually:
   ```bash
   kubectl auth can-i --list \
     --as=system:serviceaccount:node-exporter-zoneinfo:node-exporter-monitor
   ```

### CronJob Not Running

1. Check CronJob status:
   ```bash
   kubectl describe cronjob node-exporter-monitor -n node-exporter-zoneinfo
   ```

2. Check for suspended jobs:
   ```bash
   kubectl get cronjob -n node-exporter-zoneinfo -o yaml | grep suspend
   ```

3. View recent jobs:
   ```bash
   kubectl get jobs -n node-exporter-zoneinfo --sort-by=.metadata.creationTimestamp
   ```

### Reports Not Saved

1. Check PVC status:
   ```bash
   kubectl get pvc monitoring-reports -n node-exporter-zoneinfo
   ```

2. Verify mount in pod:
   ```bash
   kubectl describe pod <job-pod> -n node-exporter-zoneinfo | grep -A 5 Mounts
   ```

3. Check for write permissions:
   ```bash
   kubectl exec <job-pod> -n node-exporter-zoneinfo -- ls -la /workspace/reports
   ```
