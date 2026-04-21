package maestro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
)

const (
	// DefaultMaestroURL is the default Maestro service URL in the cluster.
	DefaultMaestroURL = "http://localhost:8000"

	//Maestro API paths
	resourceBundlesBasePath = "/api/maestro/v1/resource-bundles"
	consumersBasePath       = "/api/maestro/v1/consumers"
)

// toJSONBLabelSearch converts a label key=value pair into Maestro's JSONB search syntax.
// Maestro stores ManifestWork labels inside the payload JSONB column at payload->'metadata'->'labels'.
// The REST API's "search" parameter supports PostgreSQL JSONB containment queries (@>).
func toJSONBLabelSearch(key, value string) string {
	return fmt.Sprintf("payload->'metadata'->'labels'@>'{%q:%q}'", key, value)
}

// Client provides methods to interact with the Maestro API
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Maestro API client
// If baseURL is empty, it tries the following in order:
//  1. MAESTRO_URL environment variable
//  2. Auto-discovery from Kubernetes cluster (if available)
//  3. Default in-cluster service URL
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = os.Getenv("MAESTRO_URL")
		if baseURL == "" {
			// Try to discover from cluster, fall back to default if discovery fails
			if discovered, err := DiscoverMaestroURL(); err == nil && discovered != "" {
				baseURL = discovered
			} else {
				baseURL = DefaultMaestroURL
			}
		}
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetResourceBundles retrieves all resource bundles from Maestro
func (c *Client) GetResourceBundles(ctx context.Context) (*ResourceBundleList, error) {
	reqURL := fmt.Sprintf("%s%s", c.baseURL, resourceBundlesBasePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var result ResourceBundleList
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetResourceBundle retrieves a specific resource bundle by ID
func (c *Client) GetResourceBundle(ctx context.Context, id string) (*ResourceBundle, error) {
	reqURL := fmt.Sprintf("%s%s/%s", c.baseURL, resourceBundlesBasePath, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var result ResourceBundle
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// DeleteResourceBundle deletes a resource bundle by ID
func (c *Client) DeleteResourceBundle(ctx context.Context, id string) error {
	reqURL := fmt.Sprintf("%s%s/%s", c.baseURL, resourceBundlesBasePath, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// FindResourceBundleByClusterID finds a resource bundle by cluster ID label
// Uses server-side filtering via Maestro's search parameter with JSONB syntax
func (c *Client) FindResourceBundleByClusterID(ctx context.Context, clusterID string) (*ResourceBundle, error) {
	apiURL := fmt.Sprintf("%s%s?search=%s",
		c.baseURL,
		resourceBundlesBasePath,
		url.QueryEscape(toJSONBLabelSearch(client.KeyClusterID, clusterID)))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var result ResourceBundleList
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("no resource bundle found for cluster ID: %s", clusterID)
	}

	// Verify the result matches our cluster ID (defense in depth)
	for i := range result.Items {
		if result.Items[i].Metadata.Labels != nil &&
			result.Items[i].Metadata.Labels[client.KeyClusterID] == clusterID {
			return &result.Items[i], nil
		}
	}

	return nil, fmt.Errorf("no resource bundle found for cluster ID: %s", clusterID)
}

// FindAllResourceBundlesByClusterID finds all resource bundles for a cluster ID
// Returns all matching resource bundles (multiple adapters may create ManifestWorks for the same cluster)
func (c *Client) FindAllResourceBundlesByClusterID(ctx context.Context, clusterID string) ([]ResourceBundle, error) {
	apiURL := fmt.Sprintf("%s%s?search=%s",
		c.baseURL,
		resourceBundlesBasePath,
		url.QueryEscape(toJSONBLabelSearch(client.KeyClusterID, clusterID)))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var result ResourceBundleList
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Filter and return all matching resource bundles
	var bundles []ResourceBundle
	for i := range result.Items {
		if result.Items[i].Metadata.Labels != nil &&
			result.Items[i].Metadata.Labels[client.KeyClusterID] == clusterID {
			bundles = append(bundles, result.Items[i])
		}
	}

	return bundles, nil
}

// FindResourceBundlesByAdapterName finds all resource bundles created by a specific adapter
// Uses the maestro.io/source-id label to filter by adapter name
func (c *Client) FindResourceBundlesByAdapterName(ctx context.Context, adapterName string) ([]ResourceBundle, error) {
	apiURL := fmt.Sprintf("%s%s?search=%s",
		c.baseURL,
		resourceBundlesBasePath,
		url.QueryEscape(toJSONBLabelSearch("maestro.io/source-id", adapterName)))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var result ResourceBundleList
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Filter and return all matching resource bundles
	var bundles []ResourceBundle
	for i := range result.Items {
		if result.Items[i].Metadata.Labels != nil &&
			result.Items[i].Metadata.Labels["maestro.io/source-id"] == adapterName {
			bundles = append(bundles, result.Items[i])
		}
	}

	return bundles, nil
}

// ListConsumers retrieves the list of registered Maestro consumers
// Returns a list of consumer names
func (c *Client) ListConsumers(ctx context.Context) ([]string, error) {
	reqURL := fmt.Sprintf("%s%s", c.baseURL, consumersBasePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var result ConsumerList
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract consumer names from the response
	names := make([]string, len(result.Items))
	for i, consumer := range result.Items {
		names[i] = consumer.Name
	}

	return names, nil
}
