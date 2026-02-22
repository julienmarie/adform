package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"adform/internal/landing"
	"adform/internal/workspace"
)

type k8sOptions struct {
	Account   string
	Root      string
	OutDir    string
	Namespace string
	Image     string
	Force     bool
}

func runK8s(_ context.Context, args []string, stdout, stderr io.Writer) int {
	opts := k8sOptions{}
	fs := flag.NewFlagSet("k8s", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.Account, "account", "", "Account/property name")
	fs.StringVar(&opts.Root, "root", ".", "Repo root")
	fs.StringVar(&opts.OutDir, "out", "", "Output directory for manifests (default <account>/landing/k8s)")
	fs.StringVar(&opts.Namespace, "namespace", "default", "Kubernetes namespace")
	fs.StringVar(&opts.Image, "image", "ghcr.io/example/adform:latest", "Container image")
	fs.BoolVar(&opts.Force, "force", false, "Overwrite existing files")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if err := ensureAccount(opts.Account); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	loaded, err := landing.Load(opts.Root, opts.Account, landing.ServeOptions{Root: opts.Root, Account: opts.Account, Env: "prod"})
	if err != nil {
		fmt.Fprintf(stderr, "error: load landing config: %v\n", err)
		return 1
	}

	stateFile := fmt.Sprintf("landing_state_%s.db", opts.Account)
	outDir := strings.TrimSpace(opts.OutDir)
	if outDir == "" {
		outDir = filepath.Join(opts.Account, "landing", "k8s")
	}
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(opts.Root, outDir)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "error: create out dir: %v\n", err)
		return 1
	}

	files := map[string]string{
		"deployment.yaml":     renderLandingDeploymentYAML(opts.Namespace, opts.Image, opts.Account, stateFile),
		"service.yaml":        renderLandingServiceYAML(opts.Namespace),
		"configmap.yaml":      renderLandingConfigMapYAML(opts.Namespace, loaded.SitePath),
		"secret.example.yaml": renderLandingSecretExampleYAML(opts.Namespace),
		"README.md":           renderLandingK8sREADME(opts.Account),
	}
	for name, content := range files {
		path := filepath.Join(outDir, name)
		if !opts.Force {
			if _, err := os.Stat(path); err == nil {
				continue
			}
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			fmt.Fprintf(stderr, "error: write %s: %v\n", path, err)
			return 1
		}
	}

	fmt.Fprintf(stdout, "k8s manifests written to %s\n", outDir)
	return 0
}

func renderLandingDeploymentYAML(namespace, image, account, stateFile string) string {
	landingRoot := filepath.ToSlash(workspace.AccountLandingDir("/app", account))
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: adform-landing
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: adform-landing
  template:
    metadata:
      labels:
        app: adform-landing
    spec:
      containers:
        - name: adform
          image: %s
          imagePullPolicy: IfNotPresent
          args:
            - serve
            - --account
            - %s
            - --root
            - /app
            - --state
            - /tmp/%s
          ports:
            - name: http
              containerPort: 8080
          env:
            - name: ADFORM_SERVER_ENV
              value: prod
            - name: ADFORM_SERVER_BIND
              value: 0.0.0.0:8080
            - name: ADFORM_SERVER_ACCOUNT
              value: %s
            - name: POSTHOG_API_KEY
              valueFrom:
                secretKeyRef:
                  name: adform-landing-secrets
                  key: POSTHOG_API_KEY
            - name: META_CAPI_TOKEN
              valueFrom:
                secretKeyRef:
                  name: adform-landing-secrets
                  key: META_CAPI_TOKEN
          volumeMounts:
            - name: landing-site
              mountPath: %s/site.yml
              subPath: site.yml
      volumes:
        - name: landing-site
          configMap:
            name: adform-landing-site
`, namespace, image, account, stateFile, account, landingRoot)
}

func renderLandingServiceYAML(namespace string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: adform-landing
  namespace: %s
spec:
  selector:
    app: adform-landing
  ports:
    - name: http
      port: 80
      targetPort: http
`, namespace)
}

func renderLandingConfigMapYAML(namespace, sitePath string) string {
	site, _ := os.ReadFile(sitePath)
	if len(site) == 0 {
		site = []byte("version: 1\n")
	}
	body := strings.ReplaceAll(string(site), "\n", "\n    ")
	return fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: adform-landing-site
  namespace: %s
data:
  site.yml: |
    %s
`, namespace, body)
}

func renderLandingSecretExampleYAML(namespace string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: adform-landing-secrets
  namespace: %s
type: Opaque
stringData:
  POSTHOG_API_KEY: ""
  META_CAPI_TOKEN: ""
  REDIS_PASSWORD: ""
`, namespace)
}

func renderLandingK8sREADME(account string) string {
	return fmt.Sprintf("# Landing K8s Manifests\n\nGenerated by:\n\n`adform k8s --account %s`\n\n## Notes\n- Generates core runtime manifests only: Deployment + Service + ConfigMap + Secret template.\n- No Ingress manifest is generated.\n- No PVC manifest is generated (state DB uses container `/tmp`).\n- Optional Redis password secret key is included (`REDIS_PASSWORD`) for bandit Redis backend.\n- Expects pages/theme/assets baked in image under `/app/%s/landing`.\n\n## Apply\n\n```bash\nkubectl apply -f %s/landing/k8s/\n```\n", account, account, account)
}
