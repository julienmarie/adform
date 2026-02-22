# Kubernetes Runtime Example

This manifest set deploys the landing server using a prebuilt data image.

## Customize Before Apply

Update `deployment.yaml`:
- `image: ghcr.io/your-org/adform-data:deploy-<data-sha>`
- `ADFORM_SERVER_ACCOUNT` if not `btd_main`

## Apply

```bash
kubectl apply -k deploy/k8s
```

## Notes

- No Ingress is included.
- State volume is `emptyDir` by default.
- To persist state, replace `emptyDir` with PVC-backed storage.
