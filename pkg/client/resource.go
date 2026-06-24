package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

const apiPrefix = "/api/hyperfleet/v1/"

type Resource struct {
	Id              *string           `json:"id,omitempty"`
	Kind            string            `json:"kind"`
	Name            string            `json:"name"`
	Href            *string           `json:"href,omitempty"`
	Spec            map[string]any    `json:"spec,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Generation      int32             `json:"generation"`
	CreatedTime     *time.Time        `json:"created_time,omitempty"`
	UpdatedTime     *time.Time        `json:"updated_time,omitempty"`
	DeletedTime     *time.Time        `json:"deleted_time,omitempty"`
	CreatedBy       *string           `json:"created_by,omitempty"`
	UpdatedBy       *string           `json:"updated_by,omitempty"`
	DeletedBy       *string           `json:"deleted_by,omitempty"`
	ResourceVersion *string           `json:"resource_version,omitempty"`
}

type ResourceList struct {
	Items []Resource `json:"items"`
	Total int32      `json:"total"`
	Size  int32      `json:"size"`
	Page  int32      `json:"page"`
}

type ResourceCreateRequest struct {
	Kind   string            `json:"kind"`
	Name   string            `json:"name"`
	Spec   map[string]any    `json:"spec"`
	Labels map[string]string `json:"labels,omitempty"`
}

type ResourcePatchRequest struct {
	Spec   map[string]any    `json:"spec,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
}

func (c *HyperFleetClient) CreateResource(ctx context.Context, path string, body any) (*Resource, error) {
	resp, err := c.doJSON(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, fmt.Errorf("create resource at %s: %w", path, err)
	}
	return handleHTTPResponse[Resource](resp, http.StatusCreated, "create resource at "+path)
}

func (c *HyperFleetClient) GetResource(ctx context.Context, path string) (*Resource, error) {
	resp, err := c.doJSON(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("get resource at %s: %w", path, err)
	}
	return handleHTTPResponse[Resource](resp, http.StatusOK, "get resource at "+path)
}

func (c *HyperFleetClient) ListResources(ctx context.Context, path string, search string) (*ResourceList, error) {
	if search != "" {
		path += "?search=" + url.QueryEscape(search)
	}
	resp, err := c.doJSON(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("list resources at %s: %w", path, err)
	}
	return handleHTTPResponse[ResourceList](resp, http.StatusOK, "list resources at "+path)
}

func (c *HyperFleetClient) DeleteResource(ctx context.Context, path string) (*Resource, error) {
	resp, err := c.doJSON(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, fmt.Errorf("delete resource at %s: %w", path, err)
	}
	return handleHTTPResponse[Resource](resp, http.StatusAccepted, "delete resource at "+path)
}

func (c *HyperFleetClient) PatchResource(ctx context.Context, path string, body any) (*Resource, error) {
	resp, err := c.doJSON(ctx, http.MethodPatch, path, body)
	if err != nil {
		return nil, fmt.Errorf("patch resource at %s: %w", path, err)
	}
	return handleHTTPResponse[Resource](resp, http.StatusOK, "patch resource at "+path)
}

func (c *HyperFleetClient) CreateResourceFromPayload(ctx context.Context, path string, payloadPath string) (*Resource, error) {
	logger.Debug("loading resource payload", "path", path, "payload_path", payloadPath)

	payload, err := loadPayloadFromFile[map[string]any](payloadPath)
	if err != nil {
		logger.Error("failed to load payload", "payload_path", payloadPath, "error", err)
		return nil, fmt.Errorf("load resource payload %s: %w", payloadPath, err)
	}

	return c.CreateResource(ctx, path, payload)
}

func (c *HyperFleetClient) doJSON(ctx context.Context, method, path string, body any) (*http.Response, error) {
	fullURL := c.baseURL + apiPrefix + path

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}
