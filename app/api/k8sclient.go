package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type K8sClient struct {
	baseURL   string
	token     string
	namespace string
	httpc     *http.Client
}

func NewInClusterK8sClient() (*K8sClient, error) {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, fmt.Errorf("KUBERNETES_SERVICE_HOST/PORT not set")
	}
	token, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return nil, fmt.Errorf("read token: %w", err)
	}
	ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return nil, fmt.Errorf("read namespace: %w", err)
	}
	ca, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("read ca.crt: %w", err)
	}
	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(ca); !ok {
		return nil, fmt.Errorf("append ca cert failed")
	}
	tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}
	httpc := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	return &K8sClient{
		baseURL:   "https://" + host + ":" + port,
		token:     strings.TrimSpace(string(token)),
		namespace: strings.TrimSpace(string(ns)),
		httpc:     httpc,
	}, nil
}

func (kc *K8sClient) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, kc.baseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+kc.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return kc.httpc.Do(req)
}

func (kc *K8sClient) doRaw(ctx context.Context, method, path, contentType string, data []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, kc.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+kc.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", contentType)
	return kc.httpc.Do(req)
}

func (kc *K8sClient) CreateImageEffectJob(ctx context.Context, image, redisAddr, postID, effect string) (string, error) {
	suffix, _ := randomID(4)
	name := fmt.Sprintf("imgfx-%s-%s", strings.ToLower(postID), strings.ToLower(suffix))
	tmplPath := getenv("JOB_TEMPLATE_PATH", "/app/job.yaml")
	data, err := os.ReadFile(tmplPath)
	if err != nil {
		return "", fmt.Errorf("read job template: %w", err)
	}
	yaml := strings.NewReplacer(
		"{{NAME}}", name,
		"{{NAMESPACE}}", kc.namespace,
		"{{IMAGE}}", image,
		"{{REDIS_ADDR}}", redisAddr,
		"{{POST_ID}}", postID,
		"{{EFFECT}}", effect,
	).Replace(string(data))
	resp, err := kc.doRaw(ctx, http.MethodPost, "/apis/batch/v1/namespaces/"+kc.namespace+"/jobs", "application/yaml", []byte(yaml))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create job http %d: %s", resp.StatusCode, string(b))
	}
	return name, nil
}

func (kc *K8sClient) JobStatus(ctx context.Context, name string) (string, string, error) {
	resp, err := kc.do(ctx, http.MethodGet, "/apis/batch/v1/namespaces/"+kc.namespace+"/jobs/"+name, nil)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("get job http %d: %s", resp.StatusCode, string(b))
	}
	var doc struct {
		Status struct {
			Succeeded  *int `json:"succeeded,omitempty"`
			Failed     *int `json:"failed,omitempty"`
			Active     *int `json:"active,omitempty"`
			Conditions []struct {
				Type    string `json:"type"`
				Status  string `json:"status"`
				Reason  string `json:"reason,omitempty"`
				Message string `json:"message,omitempty"`
			} `json:"conditions,omitempty"`
		} `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", "", err
	}
	if doc.Status.Succeeded != nil && *doc.Status.Succeeded > 0 {
		return "succeeded", "", nil
	}
	if doc.Status.Failed != nil && *doc.Status.Failed > 0 {
		for _, c := range doc.Status.Conditions {
			if strings.EqualFold(c.Type, "Failed") && strings.EqualFold(c.Status, "True") {
				return "failed", firstNonEmpty(c.Reason, c.Message), nil
			}
		}
		return "failed", "", nil
	}
	for _, c := range doc.Status.Conditions {
		if strings.EqualFold(c.Type, "Complete") && strings.EqualFold(c.Status, "True") {
			return "succeeded", "", nil
		}
		if strings.EqualFold(c.Type, "Failed") && strings.EqualFold(c.Status, "True") {
			return "failed", firstNonEmpty(c.Reason, c.Message), nil
		}
	}
	if doc.Status.Active != nil && *doc.Status.Active > 0 {
		return "running", "", nil
	}
	return "pending", "", nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
