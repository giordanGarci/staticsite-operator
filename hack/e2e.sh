#!/usr/bin/env bash
# End-to-end check against a real cluster: create a StaticSite, curl it, change it, delete it.
set -euo pipefail

NS=default
SITE=hello

fetch() {
  kubectl run "curl-$RANDOM" -n "$NS" --rm -i --restart=Never --quiet \
    --image=curlimages/curl:8.10.1 -- curl -sf "http://$SITE.$NS.svc.cluster.local"
}

echo "==> waiting for the operator"
kubectl -n staticsite-system rollout status deployment/staticsite-controller-manager --timeout=120s

echo "==> creating StaticSite"
kubectl apply -n "$NS" -f config/samples/sites_v1alpha1_staticsite.yaml
kubectl wait -n "$NS" --for=condition=Ready "staticsite/$SITE" --timeout=180s
kubectl get -n "$NS" staticsite "$SITE"
fetch | grep -q "Hello from a Kubernetes operator" && echo "ok: initial content served"

echo "==> updating content (expects a rolling update)"
kubectl patch -n "$NS" staticsite "$SITE" --type=merge -p '{"spec":{"content":"<h1>updated</h1>"}}'
kubectl -n "$NS" rollout status "deployment/$SITE" --timeout=120s
for _ in $(seq 1 20); do
  if fetch | grep -q "updated"; then echo "ok: updated content served"; break; fi
  sleep 3
done
fetch | grep -q "updated"

echo "==> scaling through the scale subresource"
kubectl scale -n "$NS" "staticsite/$SITE" --replicas=3
kubectl wait -n "$NS" "deployment/$SITE" --for=jsonpath='{.status.readyReplicas}'=3 --timeout=120s
echo "ok: scaled to 3"

echo "==> deleting StaticSite (owned objects must be garbage collected)"
kubectl delete -n "$NS" staticsite "$SITE"
kubectl wait -n "$NS" --for=delete "deployment/$SITE" "service/$SITE" "configmap/$SITE" --timeout=120s
echo "ok: owned resources removed"

echo "e2e passed"
